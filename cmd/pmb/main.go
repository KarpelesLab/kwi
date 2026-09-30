// Command pmb inspects/edits the pmb boot-parameter block (a KWI component).
//
//	pmb info      <pmb>
//	pmb setcmdline <pmb-in> <pmb-out> "<cmdline>"
//	pmb setkernel  <pmb-in> <pmb-out> <xipImage> [loadaddr-hex]
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/KarpelesLab/kwi/pmb"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "info":
		info(need(1))
	case "setcmdline":
		setcmdline(need(3))
	case "setkernel":
		setkernel(nAtLeast(3))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pmb - boot-parameter block tool (a KWI component; get it via 'kwi extract')

  pmb info       <pmb>
  pmb setcmdline <pmb-in> <pmb-out> "<cmdline>"
  pmb setkernel  <pmb-in> <pmb-out> <xipImage> [loadaddr-hex]
`)
	os.Exit(2)
}

func need(n int) []string {
	if len(os.Args) != 2+n {
		usage()
	}
	return os.Args[2:]
}
func nAtLeast(n int) []string {
	if len(os.Args) < 2+n {
		usage()
	}
	return os.Args[2:]
}
func die(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, "pmb:", e)
		os.Exit(1)
	}
}
func rd(p string) []byte { b, e := os.ReadFile(p); die(e); return b }

func info(a []string) {
	p, err := pmb.Parse(rd(a[0]))
	die(err)
	fmt.Printf("version/variant : %s / %s\n", p.Version(), p.Variant())
	fmt.Printf("kernel          : load 0x%08x  size 0x%x  crc32 0x%08x\n", p.KernelLoad(), p.KernelSize(), p.KernelCRC())
	fmt.Printf("rootfs          : base 0x%08x  size 0x%x\n", p.RootfsBase(), p.RootfsSize())
	fmt.Printf("usrconf         : load 0x%08x  size 0x%x  crc 0x%08x\n", p.UsrconfLoad(), p.UsrconfSize(), p.UsrconfCRC())
	fmt.Printf("hcrc (0x04)     : 0x%08x  (algorithm not yet reversed)\n", p.HCRC())
	fmt.Printf("cmdline         : %s\n", p.Cmdline())
}

func setcmdline(a []string) {
	p, err := pmb.Parse(rd(a[0]))
	die(err)
	die(p.SetCmdline(a[2]))
	die(os.WriteFile(a[1], p.Bytes(), 0o644))
	warn()
	fmt.Printf("wrote %s with new cmdline\n", a[1])
}

func setkernel(a []string) {
	p, err := pmb.Parse(rd(a[0]))
	die(err)
	p.SetKernelImage(rd(a[2]))
	if len(a) >= 4 {
		v, err := strconv.ParseUint(a[3], 0, 32)
		die(err)
		p.SetKernelLoad(uint32(v))
	}
	die(os.WriteFile(a[1], p.Bytes(), 0o644))
	warn()
	fmt.Printf("wrote %s: kernel size 0x%x crc32 0x%08x\n", a[1], p.KernelSize(), p.KernelCRC())
}

func warn() {
	fmt.Fprintln(os.Stderr, "pmb: NOTE header CRC (0x04) is preserved, not recomputed — "+
		"if the resident U-Boot verifies it, this pmb may be rejected until that algorithm is reversed.")
}
