# KWI container format

`LOADING.KWI` is the packaged program image used by Panasonic/Aisin AW factory
navigation head units built on the Renesas R-Car H1 (`R8A77791`) platform — e.g.
Toyota `86100-5818x` / model `CQ-UT24J0AJ` (30-series Alphard/Vellfire, JBL), and
the shared Land Cruiser / Lexus modules (`86421-60V…`). It is the unit of the
**software-update ("reprogram" / リプロ) flow**: a resident updater copies a KWI's
payload into internal storage, showing the baked-in bitmap screens
("新しいソフトウェアをコピー中です") while it runs.

This documents the **program/OS container**. For the *map* data format on these units
see the separate, maps-focused project https://github.com/jharg/kiwiread .

Reverse-engineered from JDM 2015 and 2021 map SD images; all findings below verified
across 12 KWI variants (`HC02 HC11 HC59 HC60 HD70 HD72 HE38 HF06 HF93 HH40 HJ66`, plus a
`BKPRG` backup copy) and both a 2015 and 2021 build.

## Overall layout

A KWI is three concatenated sections with a small fixed header:

```
offset                       size                         section
------------------------     -------------------------    -----------------------------------------
0x000000                     0x158002  (1,409,026)        FRONT  = header + updater/loader + UI bitmaps
0x158002                     0xFF00000 (267,386,880)      ROOT   = ext2 filesystem, EXACTLY 255 MiB
0x10058002 .. EOF            variable  (~4 MB)            KERNEL = raw uncompressed Linux 2.6.35 image
```

- **FRONT offset is constant** (`0x158002`) on every observed image, old and new.
- **ROOT is a fixed 255 MiB region** (`0xFF00000`, ext2, block size 4096, 65,280 blocks).
  The ext2 filesystem fills the region exactly; its size is also readable from the ext2
  superblock (`blocks_count * block_size`).
- **KERNEL is the tail**: everything from `FRONT + ROOT` to EOF. Its length is the only
  thing that varies between images (per kernel build). It is an *uncompressed* ARM Linux
  image — the banner string `Linux version 2.6.35.14+ … MontaVista … SMP PREEMPT` is in
  plaintext near its end.
- **No trailing signature/footer**: the last bytes are zero padding.

## Header (first 0x20 bytes)

| offset | size | value (observed)     | meaning                                               |
|--------|------|----------------------|-------------------------------------------------------|
| 0x00   | 4    | `00 01 00 00`        | format/version marker (constant)                      |
| 0x04   | 4    | `0f 56 a3 00`        | constant across all variants (loader-related; not this file's size) |
| 0x08   | 4    | `3c 3c 8a 00`        | constant across all variants                          |
| 0x0c   | 4    | `07 00 01 66`        | constant                                              |
| 0x10   | 4    | `00 01 00 00`        | constant                                              |
| 0x14   | 4    | `01 00 00 00`        | constant                                              |
| 0x18   | 4..8 | ASCII, e.g. `HC59`   | **module tag** = the `EXE/<TAG>/` directory name      |

The header does **not** encode the section sizes: FRONT is a fixed offset, ROOT is a
fixed 255 MiB, and KERNEL is the remainder — so the updater derives all boundaries
without a size table. Only the 4-char tag and the kernel/rootfs contents differ between
variants.

## Notes for rebuilding a modified image

- Keep ROOT **exactly 255 MiB**. Modify the ext2 in place (loop-mount or `debugfs`/`e2tools`),
  do not grow it. Then `front || modified_root || kernel` reproduces a valid container byte-layout.
- The FRONT region embeds the tag/version as ASCII in several places (e.g. `"59"`, and a
  build char) and repeated data blocks; these are cosmetic version strings, **not** checksums
  of the payload (they are identical across repeated blocks and independent of ROOT/KERNEL).
- **OPEN QUESTION (must verify before flashing homebrew):** whether the *resident* updater
  (in the head unit's NOR / internal flash — not in the KWI itself) validates the package
  (CRC / hash / signature) before writing it. Nothing in the KWI header or footer looks like a
  whole-image checksum, but the resident updater has not yet been dumped. Test on a **donor**
  unit, never the car, until this is settled.

## Reprogram UI bitmaps

The update screens are **baked bitmaps** in the FRONT region (not strings), stored as raw
uncompressed 8-bpp BMPs. The two full-screen 832×496 frames:
- `ソフトウェア更新 / 新しいソフトウェアをコピー中です / 電源を切らないでください` (@ front `0x4e0a6`)
- `プログラム読込み中　電源を切らないで下さい` + microSD graphic (@ front `0xb2dca`)

The full HMI asset set (~2929 named `.bmp`) lives in the ext2 rootfs, served by the
`xe_rom_*` API in `/vns/bin/xe.so` from the runtime-mounted resource ROM `/vns/hmidesign/p0`
(a custom packed format, out of scope here).
