// Package kwi reads and writes LOADING.KWI program-container images used by
// Panasonic/Aisin AW factory navigation head units on the Renesas R-Car H1
// platform (e.g. Toyota 86100-5818x / CQ-UT24J0AJ).
//
// A KWI wraps a manifest that lists named components (bitmaps, boot params, the
// pcrd/ext2 rootfs, settings, and the kernel) followed by their concatenated
// data. The manifest is the authoritative layout. See FORMAT.md.
package kwi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	headerLen = 0x20
	// manifestScanLimit bounds the search for the manifest table.
	manifestScanLimit = 1 << 20
	maxEntries        = 64
	maxNameLen        = 64
)

// Header is the fixed 0x20-byte wrapper header at the start of a KWI.
type Header struct {
	Raw [headerLen]byte
	Tag string // module tag at 0x18, e.g. "HC59"
}

// Entry is one manifest component.
type Entry struct {
	Name   string
	Offset uint32 // relative to the data section
	Size   uint32
}

// Image is a parsed KWI.
type Image struct {
	Header       Header
	Preamble     []byte  // bytes 0 .. manifest table start (wrapper header, GraphicDB flash image, Program Block records)
	ManifestOff  int     // file offset of the first manifest entry
	DataOff      int     // file offset where the concatenated component data begins
	Entries      []Entry // manifest entries, in file order
	data         []byte  // whole file
}

var (
	errNoManifest = errors.New("kwi: manifest not found")
)

// Parse reads a whole KWI and parses its manifest.
func Parse(data []byte) (*Image, error) {
	var h Header
	if len(data) >= headerLen {
		copy(h.Raw[:], data[:headerLen])
		h.Tag = readTag(data[0x18:0x20])
	}
	moff, dataOff, ents, err := findManifest(data)
	if err != nil {
		return nil, err
	}
	return &Image{
		Header:      h,
		Preamble:    data[:moff],
		ManifestOff: moff,
		DataOff:     dataOff,
		Entries:     ents,
		data:        data,
	}, nil
}

// ParseReader reads and parses a KWI from a stream.
func ParseReader(r io.Reader) (*Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// EntryData returns the bytes of the named component.
func (img *Image) EntryData(name string) ([]byte, bool) {
	for _, e := range img.Entries {
		if e.Name == name {
			s := img.DataOff + int(e.Offset)
			return img.data[s : s+int(e.Size)], true
		}
	}
	return nil, false
}

// parseEntriesAt parses the [relOff:4 BE][size:4 BE][nameLen:2 BE][name][pad→even]
// entry chain starting at off. It returns the entries and the data-section start
// (== just past the last name). It requires offsets to be contiguous from 0.
func parseEntriesAt(data []byte, off int) ([]Entry, int, bool) {
	p := off
	var ents []Entry
	var cum uint32
	for len(ents) < maxEntries {
		if p+10 > len(data) {
			break
		}
		relOff := binary.BigEndian.Uint32(data[p:])
		size := binary.BigEndian.Uint32(data[p+4:])
		nl := int(binary.BigEndian.Uint16(data[p+8:]))
		if nl == 0 || nl > maxNameLen || p+10+nl > len(data) {
			break
		}
		name := data[p+10 : p+10+nl]
		if !printable(name) {
			break
		}
		if relOff != cum { // must be contiguous
			return nil, 0, false
		}
		ents = append(ents, Entry{Name: string(name), Offset: relOff, Size: size})
		cum += size
		p += 10 + nl
		if p&1 == 1 { // 2-byte align names
			p++
		}
		// End: next slot is not a valid entry and the data section fits the file.
		if p+10 > len(data) || !looksLikeEntry(data, p) {
			return ents, p, len(ents) >= 2 && off+0 <= len(data) && p+int(cum) <= len(data)
		}
	}
	return ents, p, len(ents) >= 2
}

func looksLikeEntry(data []byte, p int) bool {
	if p+10 > len(data) {
		return false
	}
	nl := int(binary.BigEndian.Uint16(data[p+8:]))
	if nl == 0 || nl > maxNameLen || p+10+nl > len(data) {
		return false
	}
	return printable(data[p+10 : p+10+nl])
}

// findManifest scans for the manifest: a contiguous entry chain whose first
// entry has relative offset 0 and whose data section ends at (or near) EOF.
func findManifest(data []byte) (manifestOff, dataOff int, ents []Entry, err error) {
	limit := manifestScanLimit
	if limit > len(data)-10 {
		limit = len(data) - 10
	}
	for i := 0; i < limit; i++ {
		// first entry must have relOff == 0
		if binary.BigEndian.Uint32(data[i:]) != 0 {
			continue
		}
		if !looksLikeEntry(data, i) {
			continue
		}
		e, d, ok := parseEntriesAt(data, i)
		if !ok {
			continue
		}
		var total uint32
		for _, en := range e {
			total += en.Size
		}
		// data section must fit and land at EOF within a small pad
		endPad := len(data) - (d + int(total))
		if endPad < 0 || endPad > 4096 {
			continue
		}
		return i, d, e, nil
	}
	return 0, 0, nil, errNoManifest
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func readTag(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}

// Pack rebuilds the KWI from the preamble and entries, rewriting the manifest
// table (offsets/sizes) and concatenating the component data. entryData maps a
// name to replacement bytes; entries not present keep their original data.
func (img *Image) Pack(entryData map[string][]byte) ([]byte, error) {
	// Rebuild the manifest table bytes and the data section.
	var table, blob []byte
	var cum uint32
	for _, e := range img.Entries {
		d, ok := entryData[e.Name]
		if !ok {
			d, _ = img.EntryData(e.Name)
		}
		var rec [10]byte
		binary.BigEndian.PutUint32(rec[0:], cum)
		binary.BigEndian.PutUint32(rec[4:], uint32(len(d)))
		binary.BigEndian.PutUint16(rec[8:], uint16(len(e.Name)))
		table = append(table, rec[:]...)
		table = append(table, e.Name...)
		if len(table)&1 == 1 {
			table = append(table, 0)
		}
		blob = append(blob, d...)
		cum += uint32(len(d))
	}
	// The manifest table occupies img.ManifestOff .. img.DataOff in the original.
	origTableLen := img.DataOff - img.ManifestOff
	if len(table) != origTableLen {
		return nil, fmt.Errorf("kwi: rebuilt manifest table is %d bytes, original was %d (name set changed?)", len(table), origTableLen)
	}
	// Preserve any trailing padding that followed the original data section.
	var origTotal uint32
	for _, e := range img.Entries {
		origTotal += e.Size
	}
	trailing := img.data[img.DataOff+int(origTotal):]

	out := make([]byte, 0, img.ManifestOff+len(table)+len(blob)+len(trailing))
	out = append(out, img.Preamble...) // 0 .. ManifestOff
	out = append(out, table...)
	out = append(out, blob...)
	out = append(out, trailing...)
	return out, nil
}

// Bytes returns the original file bytes.
func (img *Image) Bytes() []byte { return img.data }
