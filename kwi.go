// Package kwi reads and writes LOADING.KWI program-container images used by
// Panasonic/Aisin AW factory navigation head units on the Renesas R-Car H1
// platform (e.g. Toyota 86100-5818x / CQ-UT24J0AJ).
//
// A KWI is three concatenated sections: a fixed-size FRONT (loader + baked UI
// bitmaps), a fixed 255 MiB ROOT ext2 filesystem, and a variable KERNEL tail
// (raw uncompressed Linux). See FORMAT.md for the full specification.
package kwi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// FrontSize is the constant size of the FRONT section (header + loader + UI).
	FrontSize = 0x158002 // 1,409,026
	// RootSize is the fixed size of the ROOT ext2 section (255 MiB).
	RootSize = 0xFF00000 // 267,386,880
	// RootOffset is where the ext2 ROOT begins (== FrontSize).
	RootOffset = FrontSize
	// KernelOffset is where the KERNEL tail begins.
	KernelOffset = FrontSize + RootSize

	headerLen  = 0x20
	ext2Magic  = 0xEF53 // little-endian 53 EF, at partition+0x438
	ext2SBOff  = 0x400  // superblock starts 1024 bytes into the partition
	minKWISize = KernelOffset + 1
)

// Header holds the fixed 0x20-byte KWI header.
type Header struct {
	Raw [headerLen]byte
	Tag string // module tag at 0x18, e.g. "HC59"
}

// Image is a parsed KWI split into its three sections.
type Image struct {
	Header Header
	Front  []byte // includes the header
	Root   []byte // ext2 filesystem, len == RootSize
	Kernel []byte // raw Linux image (variable)
}

// Parse reads a whole KWI from data.
func Parse(data []byte) (*Image, error) {
	if len(data) < minKWISize {
		return nil, fmt.Errorf("kwi: too small (%d bytes, need >= %d)", len(data), minKWISize)
	}
	var h Header
	copy(h.Raw[:], data[:headerLen])
	h.Tag = readTag(data[0x18:0x20])

	// Sanity-check the ROOT ext2 superblock at the fixed offset.
	if err := checkExt2(data[RootOffset : RootOffset+ext2SBOff+0x40]); err != nil {
		return nil, fmt.Errorf("kwi: ROOT does not look like ext2 at 0x%x: %w", RootOffset, err)
	}
	return &Image{
		Header: h,
		Front:  data[:FrontSize],
		Root:   data[RootOffset:KernelOffset],
		Kernel: data[KernelOffset:],
	}, nil
}

// ParseReader reads the whole stream and parses it.
func ParseReader(r io.Reader) (*Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Pack reassembles the container: Front || Root || Kernel.
// Root must be exactly RootSize; Front must be exactly FrontSize.
func (img *Image) Pack() ([]byte, error) {
	if len(img.Front) != FrontSize {
		return nil, fmt.Errorf("kwi: Front must be %d bytes, got %d", FrontSize, len(img.Front))
	}
	if len(img.Root) != RootSize {
		return nil, fmt.Errorf("kwi: Root must be exactly %d bytes (255 MiB), got %d", RootSize, len(img.Root))
	}
	out := make([]byte, 0, len(img.Front)+len(img.Root)+len(img.Kernel))
	out = append(out, img.Front...)
	out = append(out, img.Root...)
	out = append(out, img.Kernel...)
	return out, nil
}

// WriteTo writes the packed container.
func (img *Image) WriteTo(w io.Writer) (int64, error) {
	b, err := img.Pack()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(b)
	return int64(n), err
}

// SetRoot replaces the ROOT ext2 image, enforcing the fixed size.
func (img *Image) SetRoot(root []byte) error {
	if len(root) != RootSize {
		return fmt.Errorf("kwi: root must be exactly %d bytes (255 MiB), got %d", RootSize, len(root))
	}
	if err := checkExt2(root[:ext2SBOff+0x40]); err != nil {
		return fmt.Errorf("kwi: replacement root is not valid ext2: %w", err)
	}
	img.Root = root
	return nil
}

func readTag(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}

var errNoExt2 = errors.New("ext2 superblock magic not found")

// checkExt2 verifies the ext2 magic (0xEF53) in the superblock and returns the
// filesystem size implied by the superblock.
func checkExt2(part []byte) error {
	if len(part) < ext2SBOff+0x40 {
		return errNoExt2
	}
	sb := part[ext2SBOff:]
	if binary.LittleEndian.Uint16(sb[0x38:]) != ext2Magic {
		return errNoExt2
	}
	return nil
}

// Ext2Size returns the filesystem size declared by the ROOT superblock
// (blocks_count * block_size). For a well-formed image this equals RootSize.
func (img *Image) Ext2Size() uint64 {
	sb := img.Root[ext2SBOff:]
	blocks := uint64(binary.LittleEndian.Uint32(sb[0x04:]))
	logbs := binary.LittleEndian.Uint32(sb[0x18:])
	return blocks * (1024 << logbs)
}
