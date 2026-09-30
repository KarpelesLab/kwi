// Command kwi inspects and rebuilds LOADING.KWI factory-nav program images.
//
// Usage:
//
//	kwi info    <in.KWI>
//	kwi unpack  <in.KWI> <outdir>          # writes front.bin, root.img, kernel.bin
//	kwi pack    <front.bin> <root.img> <kernel.bin> <out.KWI>
//	kwi setroot <in.KWI> <new-root.img> <out.KWI>   # replace ROOT (must be 255 MiB ext2)
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/KarpelesLab/kwi"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "info":
		info(args(1))
	case "unpack":
		unpack(args(2))
	case "pack":
		pack(args(4))
	case "setroot":
		setroot(args(3))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `kwi - read/write factory-nav LOADING.KWI program images

  kwi info    <in.KWI>
  kwi unpack  <in.KWI> <outdir>
  kwi pack    <front.bin> <root.img> <kernel.bin> <out.KWI>
  kwi setroot <in.KWI> <new-root.img> <out.KWI>
`)
	os.Exit(2)
}

func args(n int) []string {
	if len(os.Args) != 2+n {
		usage()
	}
	return os.Args[2 : 2+n]
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "kwi:", err)
		os.Exit(1)
	}
}

func read(p string) []byte {
	b, err := os.ReadFile(p)
	die(err)
	return b
}

func info(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	fmt.Printf("tag        %s\n", img.Header.Tag)
	fmt.Printf("front      0x%08x  %d bytes  (wrapper header + word-swapped NOR image: GraphicDB + loader)\n", 0, len(img.Front))
	fmt.Printf("root(ext2) 0x%08x  %d bytes  (superblock says %d)\n", img.RootOffset, len(img.Root), img.Ext2Size())
	fmt.Printf("tail       0x%08x  %d bytes  (SMNG process/task settings, then uncompressed Linux kernel)\n", img.KernelOffset(), len(img.Kernel))
	fmt.Printf("total                  %d bytes\n", len(img.Front)+len(img.Root)+len(img.Kernel))
}

func unpack(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	dir := a[1]
	die(os.MkdirAll(dir, 0o755))
	die(os.WriteFile(filepath.Join(dir, "front.bin"), img.Front, 0o644))
	die(os.WriteFile(filepath.Join(dir, "root.img"), img.Root, 0o644))
	die(os.WriteFile(filepath.Join(dir, "kernel.bin"), img.Kernel, 0o644))
	fmt.Printf("unpacked %s (tag %s) -> %s/{front.bin,root.img,kernel.bin}\n", a[0], img.Header.Tag, dir)
}

func pack(a []string) {
	img := &kwi.Image{Front: read(a[0]), Root: read(a[1]), Kernel: read(a[2])}
	out, err := img.Pack()
	die(err)
	die(os.WriteFile(a[3], out, 0o644))
	fmt.Printf("wrote %s (%d bytes)\n", a[3], len(out))
}

func setroot(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	die(img.SetRoot(read(a[1])))
	out, err := img.Pack()
	die(err)
	die(os.WriteFile(a[2], out, 0o644))
	fmt.Printf("wrote %s with replaced ROOT (%d bytes)\n", a[2], len(out))
}
