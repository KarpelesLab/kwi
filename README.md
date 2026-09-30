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

A KWI wraps a **component manifest** followed by the concatenated component data. The
manifest is the authoritative layout (offsets relative to the data section, contiguous):

| component | magic | what it is |
|---|---|---|
| `grp_dat_13cy_prgup.bmp` / `loading.bmp` | `BM` | reprogram/boot screens (baked bitmaps) |
| `pmb` | uImage | U-Boot uImage with the kernel command line |
| `rootfs` | `PCRD` | pcrd container (0x40000 header + a **255 MiB ext2** nested inside), mounted XIP |
| `USRCONF` | `SMNG` | process/task settings |
| `xipImage` | — | uncompressed (XIP) Linux 2.6.35 kernel |

Entry format: `[relOffset:4 BE][size:4 BE][nameLen:2 BE][name][pad→even]`. The kernel
cmdline (in `pmb`) shows `console=ttyS0,115200 root=/dev/pcrd rootflags=xip … pcrd=0x68000000,xip`.
See [FORMAT.md](FORMAT.md) for the full spec, the preamble (wrapper header + word-swapped
`GraphicDB V0564` flash image + Program Block records), and the open verification question.

## Install

```
go install github.com/KarpelesLab/kwi/cmd/kwi@latest
```

## Usage

```
kwi info    LOADING.KWI                        # list manifest components
kwi extract LOADING.KWI out/                   # write each component to out/
kwi replace LOADING.KWI rootfs new-rootfs out.KWI   # replace a component, repack

pcrd unpack out/rootfs ext2.img                # rootfs (PCRD) -> raw ext2
pcrd pack   ext2.img rootfs.new               # ext2 -> rootfs, full header recomputed
pcrd verify out/rootfs                         # check the per-page CRC table

pmb  info out/pmb                              # boot params: load addrs, sizes, CRCs, cmdline
pmb  setcmdline out/pmb pmb.new "<cmdline>"    # edit the kernel command line
pmb  setkernel  out/pmb pmb.new xipImage 0x62e00000   # update kernel size+crc32 (+load addr)
```

Full modify-the-rootfs workflow:

```
kwi extract LOADING.KWI out/
pcrd unpack out/rootfs ext2.img
# edit ext2.img in place (loop-mount ro/rw or debugfs); keep it the same size
pcrd pack ext2.img rootfs.new
kwi replace LOADING.KWI rootfs rootfs.new NEW.KWI
```

The `rootfs` PCRD container (a 256 KiB `crc32_le` per-page table + XIP ext2) is fully
supported: `pcrd unpack`/`pack` extract and rebuild it, recomputing the page-CRC table so
the head unit's background scrub (`pcrd_csum_thread`) finds no damage. Unmodified
round-trips are byte-identical.

## ⚠️ Warning

Whether the head unit's resident updater validates a KWI (CRC/hash/signature) before
flashing is **not yet determined**. Test only on a **donor** unit until that is settled —
a bad flash can brick the unit. This is reverse-engineering for interoperability and
repair of hardware you own; it ships no vendor firmware.

## License

MIT — see [LICENSE](LICENSE).
