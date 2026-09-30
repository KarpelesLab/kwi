# kwi

Reader/writer and format documentation for **`LOADING.KWI`** — the program/OS
container used by Panasonic/Aisin AW factory navigation head units built on the
Renesas R-Car H1 (`R8A77791`) platform.

Confirmed on Toyota `86100-5818x` / model `CQ-UT24J0AJ` (30-series Alphard/Vellfire,
JBL premium sound) and the shared Land Cruiser / Lexus navigation modules
(`86421-60V…`). These units run **MontaVista Linux 2.6.35** (with an eSOL eT-Kernel
co-OS); `LOADING.KWI` is the image the reprogram / software-update flow copies into
internal storage.

This project targets the **OS/program container**. For the *map* data on these units,
see the separate, maps-focused [`jharg/kiwiread`](https://github.com/jharg/kiwiread).

## Format

A KWI is three concatenated sections with a small fixed header:

| section | offset | contents |
|---|---|---|
| wrapper header | `0x000000` | 4 KiB, tags (`HC59`/`VC59`/`10KA`); does **not** encode the layout |
| NOR flash image | `0x001000` | partial 16-bit **word-swapped** dump of the 8 MiB x16 boot NOR: `GraphicDB V0564` archives + loader + reprogram BMPs, `0xff`-padded |
| root (ext2) | `0x158002` | ext2 filesystem, fixed **255 MiB**, the Linux rootfs (not swapped) |
| tail | `0x10058002` | `SMNG` process/task settings (text) then a raw **uncompressed** Linux 2.6.35 kernel |

The ROOT boundary is the only self-describing one (ext2 superblock); the reader derives it
from there. No trailing signature. See [FORMAT.md](FORMAT.md) for the full spec and the open
question of whether the *resident* (in-unit) updater verifies the package before flashing.

## Install

```
go install github.com/KarpelesLab/kwi/cmd/kwi@latest
```

## Usage

```
kwi info    LOADING.KWI                       # show sections
kwi unpack  LOADING.KWI out/                  # -> out/{front.bin,root.img,kernel.bin}
kwi pack    front.bin root.img kernel.bin new.KWI
kwi setroot LOADING.KWI modified-root.img new.KWI   # replace ROOT (must stay 255 MiB ext2)
```

To modify the system: `unpack`, edit `root.img` in place **keeping it exactly 255 MiB**
(loop-mount, or `debugfs` / `e2tools`), then `setroot` (or `pack`).

## ⚠️ Warning

Whether the head unit's resident updater validates a KWI (CRC/hash/signature) before
flashing is **not yet determined**. Test only on a **donor** unit until that is settled —
a bad flash can brick the unit. This is reverse-engineering for interoperability and
repair of hardware you own; it ships no vendor firmware.

## License

MIT — see [LICENSE](LICENSE).
