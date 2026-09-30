// Command kwi inspects and rebuilds LOADING.KWI factory-nav program images by
// parsing their component manifest.
//
// Usage:
//
//	kwi info    <in.KWI>                       # list manifest entries
//	kwi extract <in.KWI> <outdir>             # write every component to outdir/
//	kwi replace <in.KWI> <name> <file> <out.KWI>   # replace one component, repack
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
	case "extract":
		extract(args(2))
	case "replace":
		replace(args(4))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `kwi - read/write factory-nav LOADING.KWI program images

  kwi info    <in.KWI>
  kwi extract <in.KWI> <outdir>
  kwi replace <in.KWI> <name> <file> <out.KWI>
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

func read(p string) []byte { b, err := os.ReadFile(p); die(err); return b }

func info(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	fmt.Printf("tag          %s\n", img.Header.Tag)
	fmt.Printf("manifest at  0x%x, data at 0x%x, %d entries\n\n", img.ManifestOff, img.DataOff, len(img.Entries))
	fmt.Printf("  %-24s %-12s %-12s\n", "name", "abs-offset", "size")
	for _, e := range img.Entries {
		fmt.Printf("  %-24s 0x%08x   0x%08x (%d)\n", e.Name, img.DataOff+int(e.Offset), e.Size, e.Size)
	}
}

func extract(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	dir := a[1]
	die(os.MkdirAll(dir, 0o755))
	for _, e := range img.Entries {
		d, _ := img.EntryData(e.Name)
		// sanitize name for the filesystem
		name := filepath.Base(e.Name)
		die(os.WriteFile(filepath.Join(dir, name), d, 0o644))
		fmt.Printf("  %-24s -> %s (%d bytes)\n", e.Name, name, len(d))
	}
}

func replace(a []string) {
	img, err := kwi.Parse(read(a[0]))
	die(err)
	name, file, out := a[1], a[2], a[3]
	found := false
	for _, e := range img.Entries {
		if e.Name == name {
			found = true
		}
	}
	if !found {
		die(fmt.Errorf("no such component %q", name))
	}
	nd := read(file)
	packed, err := img.Pack(map[string][]byte{name: nd})
	die(err)
	die(os.WriteFile(out, packed, 0o644))
	fmt.Printf("replaced %q with %s (%d bytes), wrote %s (%d bytes)\n", name, file, len(nd), out, len(packed))
}
