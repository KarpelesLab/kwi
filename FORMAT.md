# KWI container format

`LOADING.KWI` is the packaged program image used by Panasonic/Aisin AW factory
navigation head units built on the Renesas R-Car H1 (`R8A77791`) platform — e.g.
Toyota `86100-5818x` / model `CQ-UT24J0AJ` (30-series Alphard/Vellfire, JBL), and
the shared Land Cruiser / Lexus modules (`86421-60V…`). It is the unit of the
**software-update ("reprogram" / リプロ) flow**: a resident updater copies a KWI's
components into internal flash, showing baked-in bitmap screens
("新しいソフトウェアをコピー中です") while it runs.

This documents the **program/OS container**. For the *map* data format on these units
see the separate, maps-focused project https://github.com/jharg/kiwiread .

Reverse-engineered from JDM 2015 and 2021 map SD images; verified across 12 KWI
variants. `<TAG>` = the `EXE/<TAG>/` directory name (e.g. `HC59`).

## Overall structure

```
0x000000  Wrapper header (0x20 bytes, not byte-swapped): tags HC59/VC59/10KA + constants
0x001000  NOR flash image (16-bit word-swapped): block table + GraphicDB V0564 archives
          + plain reprogram BMPs, 0xff-padded  (a partial dump of the 8 MiB x16 boot NOR)
0x04c000  "Program Block" records (0x400 each, repeated): module tag / date / "AISIN DEVELOP"
0x04e02e  MANIFEST  (component table)  <-- the authoritative layout
0x04e0a6  DATA      (components concatenated in manifest order)
```

Everything before the DATA section (wrapper header, GraphicDB flash image, Program
Block records, and the manifest table itself) is the "preamble". The **manifest** is
the source of truth: component offsets are relative to the DATA start and are
contiguous (each offset = sum of preceding sizes).

## Manifest entry

Each entry, starting at the first one (relative offset 0):

```
+0x00  u32 be   relative offset (from DATA start; entry[0]=0, contiguous)
+0x04  u32 be   size in bytes
+0x08  u16 be   name length N
+0x0a  N bytes  name (ASCII)
       (padded with one 0x00 if N is odd, to keep entries 2-byte aligned)
```

The DATA section begins immediately after the last entry's name (+pad).

## Components (HC59 example)

| name | abs offset | size | magic | what it is |
|---|---|---|---|---|
| `grp_dat_13cy_prgup.bmp` | 0x04e0a6 | 0x64d24 | `BM` | 832×496 "copying new software" reprogram screen |
| `loading.bmp` | 0x0b2dca | 0x65038 | `BM` | 832×496 "program loading" screen |
| `pmb` | 0x117e02 | 0x200 | `27 05 19 56` | **U-Boot uImage** wrapping the kernel command line |
| `rootfs` | 0x118002 | 0x0ff40000 | `PCRD` | **pcrd container**: 0x40000 header + a 255 MiB ext2 nested inside |
| `USRCONF` | 0x10058002 | 0xcbb0 | `SMNG_PRO` | process/task settings (tab-separated text) |
| `xipImage` | 0x10064bb2 | 0x417914 | (ARM) | uncompressed (execute-in-place) Linux 2.6.35 kernel |

### `rootfs` = PCRD (XIP ext2 with a per-page CRC table)

The `rootfs` component starts with a `PCRD` header, then the ext2. Format (verified
against the GPL driver `drivers/block/pcrd.c` — `pcrd_check_header` / `pcrd_validate_page`):

```
0x0    "PCRD"                  magic
0x4    u32 le  num_pages       255 MiB / 4096 = 65280
0x8    u32                     not validated by the driver (preserved on rebuild)
0xc    u32 le  csum[num_pages] per 4 KiB page: crc32_le(0xFFFFFFFF, page)
                               (Linux crc32_le, poly 0xEDB88320, NO final XOR
                                = zlib.crc32(page) ^ 0xFFFFFFFF)
       0xff padding to 0x40000 (256 KiB header)
0x40000 .. end                 the ext2 filesystem (num_pages * 4096)
```

Verified: all 65,280 stored CRCs match `crc32_le` of the corresponding 4 KiB page.

The driver mounts this **execute-in-place** as `/dev/pcrd` (`root=/dev/pcrd rootflags=xip`,
`pcrd=0x68000000,xip`). Reads use unchecked XIP `pcrd_direct_access`; a **background**
`pcrd_csum_thread` verifies pages and calls `pcrd_detect_damagedarea` / logs
`page validation failed` on a mismatch. So a stale table does not block boot, but the
`pcrd` tool recomputes it so the scrub finds nothing wrong.

The `pcrd` package/CLI in this repo decodes and re-encodes this container.

## Rebuilding a modified image

Parse the manifest, replace a component's bytes, then rewrite the manifest table
(recomputing each entry's offset/size) and re-concatenate the data. `kwi replace`
does this; an unmodified round-trip is byte-identical (trailing padding preserved).

To modify the Linux userland: extract `rootfs`, edit the ext2 nested inside the PCRD
container (skip the 0x40000 pcrd header; loop-mount or `debugfs`), keep the total
`rootfs` size unchanged, then `kwi replace <in> rootfs <new-rootfs> <out>`.

## Open question (verify before flashing)

The PCRD per-page table is fully understood and recomputable (above), so the rootfs is no
longer a blocker. What remains unverified is whether the head unit's *resident* reprogram
updater (in NOR / internal flash, not in the KWI) checks the KWI as a whole (the
`Program Block` records carry per-variant values that may be checksums) before writing it.
**Test on a donor unit, never the car**, until this is settled.
