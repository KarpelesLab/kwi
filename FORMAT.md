# KWI container format

`LOADING.KWI` is the packaged program image used by Panasonic/Aisin AW factory
navigation head units built on the Renesas R-Car H1 (`R8A77791`) platform — e.g.
Toyota `86100-5818x` / model `CQ-UT24J0AJ` (30-series Alphard/Vellfire, JBL), and
the shared Land Cruiser / Lexus modules (`86421-60V…`). It is the unit of the
**software-update ("reprogram" / リプロ) flow**: a resident updater copies a KWI's
payload into internal storage, showing baked-in bitmap screens
("新しいソフトウェアをコピー中です") while it runs.

This documents the **program/OS container**. For the *map* data format on these units
see the separate, maps-focused project https://github.com/jharg/kiwiread .

Reverse-engineered from JDM 2015 and 2021 map SD images; verified across 12 KWI
variants (`HC02 HC11 HC59 HC60 HD70 HD72 HE38 HF06 HF93 HH40 HJ66` + a `BKPRG` copy)
and both a 2015 and 2021 build. `<TAG>` below is the `EXE/<TAG>/` directory name.

## Overall layout

Four logical parts. Only the ROOT boundary is self-describing (via the ext2
superblock); the others are derived from it and from fixed alignment.

```
offset (HC59 example)         section
-------------------------     -------------------------------------------------------------
0x000000 .. 0x001000          WRAPPER HEADER  (4 KiB, NOT byte-swapped)
0x001000 .. ~0x157c0e         NOR FLASH IMAGE (partial dump of the 8 MiB x16 boot NOR)
        ~0x157c0e .. 0x158002 0xff erased-flash padding to the ROOT boundary
0x158002 .. 0x10058002        ROOT   = ext2 filesystem, 255 MiB (0xFF00000)   [NOT swapped]
0x10058002 .. (kernel start)  SETTINGS = "SMNG" process/task table (tab-separated text)
(kernel start) .. EOF         KERNEL  = raw *uncompressed* Linux 2.6.35 image
```

Sizes: WRAPPER+FLASH (the "front") is ~1.35 MiB and constant; ROOT is a fixed 255 MiB;
the tail (SETTINGS+KERNEL) is the remainder (~4 MiB) and its length is the only thing
that varies between variants.

### Wrapper header (0x0 .. 0x20), not byte-swapped

| offset | value (HC59)   | meaning                                              |
|--------|----------------|------------------------------------------------------|
| 0x00   | `00 01 00 00`  | format/version marker (constant)                     |
| 0x04   | `0f 56 a3 00`  | constant across all variants (purpose unconfirmed)   |
| 0x08   | `3c 3c 8a 00`  | constant across all variants                         |
| 0x0c   | `07 00 01 66`  | constant                                             |
| 0x10   | `00 01 00 00`  | constant                                             |
| 0x14   | `01 00 00 00`  | constant                                             |
| 0x18   | `HC59`         | module tag = `EXE/<TAG>/` dir name                   |
| 0x4c   | `VC59`         | build/version code (matches `…/13CY/VC59/…` path)    |
| 0x50   | `10KA`         | variant/hardware code                                |

The wrapper header does **NOT** encode the section offsets/sizes: none of the real
offsets (FLASH start, ROOT start/size, kernel offset, file size) appear as a stored
value anywhere in the front, and fields 0x04–0x14 are byte-identical across all 12
variants while the actual sizes differ. So a reader must locate ROOT by its ext2
superblock, not by a header field.

### NOR flash image (0x1000 .. ~0x157c0e), 16-bit word-swapped

A partial raw dump of the head unit's **8 MiB, x16 NOR** (Spansion S29JL064J), stored
**byte-swapped within each 16-bit word** (a `x16` flash-dump artifact): the string
`GraphicDB  V0564` reads as `rGpaihDc  BV5064` until you swap each 16-bit word. It holds:
- **`0x1000`**: a block/index table (sequential 16-bit entries with `ff` markers).
- **`0x1800`, `0x27000`**: `GraphicDB V0564` archives (build `2006-07-25`) — a legacy
  Denso/Aisin-lineage graphic asset format, stored in its native word-swapped order,
  with what looks like an RGB555 palette (runs of `0x7fff`) after each header.
- The reprogram/boot **BMP screens** (e.g. `0x4e0a6`, `0xb2dca`) — these are stored
  **plain (not swapped)**, so the front is mixed-endian by sub-region.
- Trailing `0xff` (erased flash) padding up to the ROOT boundary.

`GraphicDB V0564` is not documented publicly (as of 2026); its index/entry format is
still being reverse-engineered here.

### ROOT (ext2, 255 MiB)

Standard ext2, block size 4096, 65,280 blocks = exactly `0xFF00000`. Not swapped. This
is the Linux root filesystem (`/vns/...`, the HMI, etc.).

### SETTINGS + KERNEL (tail)

Immediately after ROOT is a **text** process/task manifest beginning `SMNG_PROCESSNAME`
(tab-separated: task name, scheduling class, priority, stack size, …). The
**uncompressed** ARM Linux kernel follows it; its `Linux version 2.6.35.14+ … MontaVista …`
banner is in plaintext. (The exact SETTINGS→KERNEL boundary is not yet pinned; treat the
whole tail as one blob when repacking.)

## Rebuilding a modified image

- Keep ROOT **exactly the same size** (255 MiB). Modify the ext2 in place (loop-mount or
  `debugfs`/`e2tools`); do not grow/shrink it. Then `front || modified_root || tail`
  reproduces a valid byte layout (`kwi pack` / `kwi setroot`; unpack→pack is byte-identical).
- FRONT and TAIL are passed through unchanged for a rootfs-only modification.
- **OPEN QUESTION (verify before flashing homebrew):** whether the *resident* updater (in
  the unit's NOR / internal flash, not in the KWI) validates the package (CRC/hash/signature)
  before writing. Nothing in the KWI itself is a whole-image checksum, and the footer is zero
  padding — but the resident updater has not been dumped. **Test on a donor unit, never the car.**
