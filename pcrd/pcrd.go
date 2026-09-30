// Package pcrd reads and writes the PCRD container that wraps the XIP ext2
// rootfs in a KWI (the `rootfs` component). Format reverse-engineered from the
// GPL Linux driver drivers/block/pcrd.c (pcrd_check_header / pcrd_validate_page):
//
//	offset 0x0  "PCRD"                     magic
//	offset 0x4  u32 le  num_pages          (e.g. 65280 = 255 MiB / 4096)
//	offset 0x8  u32                        unchecked (left as-is)
//	offset 0xc  u32 le  csum[num_pages]    per 4 KiB page:
//	                    crc32_le(0xFFFFFFFF, page)   (poly 0xEDB88320, NO final XOR)
//	            0xff padding to header size
//	header size (0x40000) .. end           the ext2 filesystem (num_pages*4096)
//
// The driver validates pages in a background thread (pcrd_csum_thread) and reads
// via unchecked XIP direct_access, so a stale table does not block boot — but
// Encode recomputes it so the scrub finds no "damaged area".
package pcrd

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	Magic      = "PCRD"
	HeaderSize = 0x40000 // 256 KiB (magic+count+table, 0xff-padded)
	PageSize   = 0x1000  // 4 KiB
	tableOff   = 0xc
)

// pageCRC returns crc32_le(0xFFFFFFFF, page) with no final inversion, matching
// the value pcrd_validate_page compares against. Go's ChecksumIEEE is the
// standard CRC-32 (init 0xFFFFFFFF, final XOR); undoing the final XOR gives the
// driver's value.
func pageCRC(page []byte) uint32 { return crc32.ChecksumIEEE(page) ^ 0xFFFFFFFF }

// Container is a parsed PCRD blob.
type Container struct {
	NumPages uint32
	Word8    uint32 // header[0x8], unchecked by the driver; preserved on Encode
	Ext2     []byte // the ext2 filesystem
}

// Decode parses a PCRD container and returns the ext2 payload. It verifies the
// magic; page-CRC verification is available via Verify.
func Decode(b []byte) (*Container, error) {
	if len(b) < HeaderSize || string(b[:4]) != Magic {
		return nil, fmt.Errorf("pcrd: bad magic or too small")
	}
	n := binary.LittleEndian.Uint32(b[4:])
	if uint64(HeaderSize)+uint64(n)*PageSize > uint64(len(b)) {
		return nil, fmt.Errorf("pcrd: num_pages %d exceeds data", n)
	}
	return &Container{
		NumPages: n,
		Word8:    binary.LittleEndian.Uint32(b[8:]),
		Ext2:     b[HeaderSize : HeaderSize+int(n)*PageSize],
	}, nil
}

// Verify recomputes every page CRC and returns the indexes that don't match the
// stored table (empty means the container is intact).
func Verify(b []byte) ([]int, error) {
	c, err := Decode(b)
	if err != nil {
		return nil, err
	}
	var bad []int
	for i := 0; i < int(c.NumPages); i++ {
		stored := binary.LittleEndian.Uint32(b[tableOff+i*4:])
		if stored != pageCRC(c.Ext2[i*PageSize:(i+1)*PageSize]) {
			bad = append(bad, i)
		}
	}
	return bad, nil
}

// Encode builds a PCRD container around ext2, recomputing the page-CRC table.
// ext2 must be a whole number of 4 KiB pages. word8 is written to header[0x8]
// (pass the value from a Decode of the original; it is not validated by pcrd).
func Encode(ext2 []byte, word8 uint32) ([]byte, error) {
	if len(ext2)%PageSize != 0 {
		return nil, fmt.Errorf("pcrd: ext2 size %d is not a multiple of %d", len(ext2), PageSize)
	}
	n := len(ext2) / PageSize
	if tableOff+n*4 > HeaderSize {
		return nil, fmt.Errorf("pcrd: %d pages need a bigger header than 0x%x", n, HeaderSize)
	}
	out := make([]byte, HeaderSize+len(ext2))
	// header: 0xff fill (matches the driver's erased-flash padding), then fields
	for i := 0; i < HeaderSize; i++ {
		out[i] = 0xff
	}
	copy(out, Magic)
	binary.LittleEndian.PutUint32(out[4:], uint32(n))
	binary.LittleEndian.PutUint32(out[8:], word8)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(out[tableOff+i*4:], pageCRC(ext2[i*PageSize:(i+1)*PageSize]))
	}
	copy(out[HeaderSize:], ext2)
	return out, nil
}
