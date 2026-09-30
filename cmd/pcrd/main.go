// Command pcrd unpacks/repacks the PCRD container that wraps the XIP ext2 rootfs.
//
//	pcrd unpack <rootfs> <ext2.img>     # extract the ext2 (verifies page CRCs)
//	pcrd pack   <ext2.img> <rootfs-in> <rootfs-out>   # rebuild, recomputing CRCs
//	pcrd verify <rootfs>
package main

import (
	"fmt"
	"os"

	"github.com/KarpelesLab/kwi/pcrd"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "unpack":
		unpack(need(2))
	case "pack":
		pack(need(2))
	case "verify":
		verify(need(1))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pcrd - PCRD (XIP ext2 rootfs) container tool

  pcrd unpack <rootfs> <ext2.img>
  pcrd pack   <ext2.img> <rootfs-out>
  pcrd verify <rootfs>
`)
	os.Exit(2)
}

func need(n int) []string {
	if len(os.Args) != 2+n {
		usage()
	}
	return os.Args[2 : 2+n]
}
func die(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, "pcrd:", e)
		os.Exit(1)
	}
}
func rd(p string) []byte { b, e := os.ReadFile(p); die(e); return b }

func unpack(a []string) {
	c, err := pcrd.Decode(rd(a[0]))
	die(err)
	bad, _ := pcrd.Verify(rd(a[0]))
	die(os.WriteFile(a[1], c.Ext2, 0o644))
	fmt.Printf("ext2: %d bytes (%d pages); table-crc=0x%08x; %d bad page CRCs\n",
		len(c.Ext2), c.NumPages, c.TableCRC, len(bad))
}

func pack(a []string) {
	out, err := pcrd.Encode(rd(a[0])) // whole PCRD header is computed from the ext2
	die(err)
	die(os.WriteFile(a[1], out, 0o644))
	fmt.Printf("wrote %s (%d bytes); full PCRD header recomputed from ext2\n", a[1], len(out))
}

func verify(a []string) {
	bad, err := pcrd.Verify(rd(a[0]))
	die(err)
	if len(bad) == 0 {
		fmt.Println("all page CRCs OK")
		return
	}
	fmt.Printf("%d page(s) fail CRC: %v%s\n", len(bad), first(bad, 20), more(bad, 20))
}

func first(x []int, n int) []int {
	if len(x) > n {
		return x[:n]
	}
	return x
}
func more(x []int, n int) string {
	if len(x) > n {
		return " ..."
	}
	return ""
}
