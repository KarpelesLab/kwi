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

// Typical values observed on R-Car H1 nav images. These are NOT used to locate
// the sections (the KWI header does not store the layout); they are only sanity
// bounds and documentation. The real ROOT offset and size are DERIVED from the
// ext2 superblock. See FORMAT.md.
const (
	// TypicalFrontSize is the front-section size seen on every observed image.
	TypicalFrontSize = 0x158002 // 1,409,026
	// TypicalRootSize is the ext2 ROOT size seen on every observed image (255 MiB).
	TypicalRootSize = 0xFF00000 // 267,386,880

	headerLen   = 0x20
	ext2Magic   = 0xEF53 // s_magic, little-endian, at superblock+0x38
	ext2SBOff   = 0x400  // ext2 superblock starts 1024 bytes into the partition
	ext2MagicAt = ext2SBOff + 0x38
	// frontSearchLimit bounds the scan for the ext2 superblock (front is ~1.4 MB).
	frontSearchLimit = 8 << 20
)

// Header holds the fixed 0x20-byte KWI header.
type Header struct {
	Raw [headerLen]byte
	Tag string // module tag at 0x18, e.g. "HC59"
}

// Image is a parsed KWI split into its three sections. RootOffset is where the
// ext2 ROOT was located (== len(Front)); it and the ROOT size are derived from
// the ext2 superblock, not from any KWI header field.
type Image struct {
	Header     Header
	Front      []byte // header + loader + UI bitmaps
	Root       []byte // ext2 filesystem (size read from its superblock)
	Kernel     []byte // raw Linux image (the remainder)
	RootOffset int    // == len(Front)
}

// KernelOffset is where the KERNEL tail begins (== len(Front)+len(Root)).
func (img *Image) KernelOffset() int { return len(img.Front) + len(img.Root) }

// Parse reads a whole KWI from data. It locates the ROOT filesystem by finding
// the ext2 superblock (its magic and declared size), then derives the FRONT
// (everything before it) and the KERNEL (everything after it). The KWI header
// itself does not encode the layout.
func Parse(data []byte) (*Image, error) {
	var h Header
	if len(data) >= headerLen {
		copy(h.Raw[:], data[:headerLen])
		h.Tag = readTag(data[0x18:0x20])
	}

	rootOff, rootSize, err := findExt2(data)
	if err != nil {
		return nil, err
	}
	if rootOff+rootSize > len(data) {
		return nil, fmt.Errorf("kwi: ext2 at 0x%x declares size %d but only %d bytes remain",
			rootOff, rootSize, len(data)-rootOff)
	}
	return &Image{
		Header:     h,
		Front:      data[:rootOff],
		Root:       data[rootOff : rootOff+rootSize],
		Kernel:     data[rootOff+rootSize:],
		RootOffset: rootOff,
	}, nil
}

// findExt2 scans the front of the image for a valid ext2 superblock and returns
// the partition start offset and the filesystem size (blocks_count * block_size)
// read from that superblock.
func findExt2(data []byte) (off, size int, err error) {
	limit := frontSearchLimit
	if limit > len(data) {
		limit = len(data)
	}
	for i := 0; i+ext2MagicAt+2 <= limit; i += 2 {
		if data[i] != 0x53 || data[i+1] != 0xEF { // s_magic bytes, LE
			continue
		}
		part := i - ext2MagicAt
		if part < 0 || part+ext2SBOff+0x40 > len(data) {
			continue
		}
		sb := data[part+ext2SBOff:]
		if binary.LittleEndian.Uint16(sb[0x38:]) != ext2Magic {
			continue
		}
		blocks := uint64(binary.LittleEndian.Uint32(sb[0x04:]))
		logbs := binary.LittleEndian.Uint32(sb[0x18:])
		if logbs > 6 || blocks == 0 || blocks > 1<<31 {
			continue
		}
		fsSize := blocks * (1024 << logbs)
		if fsSize == 0 || part+int(fsSize) > len(data) {
			continue
		}
		return part, int(fsSize), nil
	}
	return 0, 0, errNoExt2
}

// ParseReader reads the whole stream and parses it.
func ParseReader(r io.Reader) (*Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Pack reassembles the container: Front || Root || Kernel. Root must be a valid
// ext2 filesystem (the boundary is self-describing via its superblock).
func (img *Image) Pack() ([]byte, error) {
	if len(img.Root) < ext2SBOff+0x40 {
		return nil, fmt.Errorf("kwi: Root too small to be ext2 (%d bytes)", len(img.Root))
	}
	if err := checkExt2(img.Root[:ext2SBOff+0x40]); err != nil {
		return nil, fmt.Errorf("kwi: Root is not valid ext2: %w", err)
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

// SetRoot replaces the ROOT ext2 image. The replacement must be a valid ext2
// filesystem and, to preserve the on-flash layout the updater expects, the same
// size as the original ROOT. Modify the ext2 in place (do not grow/shrink it).
func (img *Image) SetRoot(root []byte) error {
	if len(root) != len(img.Root) {
		return fmt.Errorf("kwi: replacement root must be exactly %d bytes (same as original), got %d", len(img.Root), len(root))
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
