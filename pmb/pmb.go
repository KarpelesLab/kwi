// Package pmb parses and edits the "pmb" boot-parameter block — the KWI component
// the resident U-Boot reads to launch Linux. It starts with the U-Boot uImage
// magic (0x27051956, little-endian here) but is otherwise an Aisin-specific
// layout: a load-map of the other components plus the kernel command line.
//
// Layout (512 bytes, little-endian), reverse-engineered from the images:
//
//	0x000 u32   magic        0x27051956 (uImage magic value)
//	0x004 u32   hcrc         header CRC — algorithm not yet reproduced (needs the NOR U-Boot)
//	0x008 u32   kernelLoad   kernel DDR load address (e.g. 0x62e00000)
//	0x00c u32   kernelLoad2  kernelLoad + 0x180000 (second load/entry region)
//	0x010 u32   kernelSize   == xipImage size
//	0x014 u32   kernelCRC    == crc32(xipImage)  (standard CRC-32, with final XOR)
//	0x018 u32   rootfsBase   0x68000000 (PCRD XIP base)
//	0x01c u32   rootfsSize   == rootfs (PCRD) component size
//	0x020 ...   cmdline      NUL-terminated kernel command line
//	0x1ec u32   usrconfLoad  0x6010c000
//	0x1f0 u32   usrconfSize  == USRCONF size
//	0x1f4 u32   usrconfCRC   USRCONF CRC — algorithm not yet reproduced
//	0x1f8 [4]   version      e.g. "VC59"
//	0x1fc [4]   variant      e.g. "10KA"
package pmb

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	Size  = 0x200
	Magic = 0x27051956

	offMagic       = 0x000
	offHCRC        = 0x004
	offKernelLoad  = 0x008
	offKernelLoad2 = 0x00c
	offKernelSize  = 0x010
	offKernelCRC   = 0x014
	offRootfsBase  = 0x018
	offRootfsSize  = 0x01c
	offCmdline     = 0x020
	offUsrconfLoad = 0x1ec
	offUsrconfSize = 0x1f0
	offUsrconfCRC  = 0x1f4
	offVersion     = 0x1f8
	offVariant     = 0x1fc
)

// PMB wraps a pmb block. The raw bytes are kept so edits change only the fields
// you set and everything else round-trips exactly.
type PMB struct{ raw []byte }

// Parse validates the magic and wraps the block.
func Parse(b []byte) (*PMB, error) {
	if len(b) < Size {
		return nil, fmt.Errorf("pmb: too small (%d < %d)", len(b), Size)
	}
	if binary.LittleEndian.Uint32(b[offMagic:]) != Magic {
		return nil, fmt.Errorf("pmb: bad magic 0x%08x", binary.LittleEndian.Uint32(b[offMagic:]))
	}
	p := &PMB{raw: make([]byte, len(b))}
	copy(p.raw, b)
	return p, nil
}

func (p *PMB) u32(o int) uint32  { return binary.LittleEndian.Uint32(p.raw[o:]) }
func (p *PMB) put(o int, v uint32) { binary.LittleEndian.PutUint32(p.raw[o:], v) }

func (p *PMB) HCRC() uint32        { return p.u32(offHCRC) }
func (p *PMB) KernelLoad() uint32  { return p.u32(offKernelLoad) }
func (p *PMB) KernelSize() uint32  { return p.u32(offKernelSize) }
func (p *PMB) KernelCRC() uint32   { return p.u32(offKernelCRC) }
func (p *PMB) RootfsBase() uint32  { return p.u32(offRootfsBase) }
func (p *PMB) RootfsSize() uint32  { return p.u32(offRootfsSize) }
func (p *PMB) UsrconfLoad() uint32 { return p.u32(offUsrconfLoad) }
func (p *PMB) UsrconfSize() uint32 { return p.u32(offUsrconfSize) }
func (p *PMB) UsrconfCRC() uint32  { return p.u32(offUsrconfCRC) }
func (p *PMB) Version() string     { return trimz(p.raw[offVersion : offVersion+4]) }
func (p *PMB) Variant() string     { return trimz(p.raw[offVariant : offVariant+4]) }

// Cmdline returns the NUL-terminated kernel command line.
func (p *PMB) Cmdline() string {
	b := p.raw[offCmdline:]
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// SetCmdline replaces the command line (must fit before 0x1e0).
func (p *PMB) SetCmdline(s string) error {
	max := 0x1e0 - offCmdline
	if len(s)+1 > max {
		return fmt.Errorf("pmb: cmdline too long (%d > %d)", len(s)+1, max)
	}
	for i := offCmdline; i < 0x1e0; i++ {
		p.raw[i] = 0
	}
	copy(p.raw[offCmdline:], s)
	return nil
}

// SetKernelImage updates kernelSize and kernelCRC for a new xipImage. NOTE: the
// header CRC at 0x004 is NOT recomputed (algorithm unknown) — if the resident
// U-Boot enforces it, a modified pmb needs that solved first.
func (p *PMB) SetKernelImage(img []byte) {
	p.put(offKernelSize, uint32(len(img)))
	p.put(offKernelCRC, crc32.ChecksumIEEE(img)) // standard CRC-32, matches pmb[0x14]
}

// SetKernelLoad sets the kernel DDR load address (and the +0x180000 secondary).
func (p *PMB) SetKernelLoad(addr uint32) {
	p.put(offKernelLoad, addr)
	p.put(offKernelLoad2, addr+0x180000)
}

// Bytes returns the (possibly edited) 512-byte block.
func (p *PMB) Bytes() []byte { return p.raw }

// HCRCReproduced reports whether Bytes() has a valid, recomputed header CRC.
// Currently false: the hcrc/usrconfCRC algorithms are not yet reversed, so those
// fields are preserved from the original rather than recomputed.
func (p *PMB) HCRCReproduced() bool { return false }

func trimz(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
