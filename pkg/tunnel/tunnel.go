package tunnel

type Tunnel struct {
	Src       string
	SrcServer string
	DstServer string
	Dst       string
}

func New(src, srcServer, dstServer, dst string) Tunnel {
	return Tunnel{Src: src, SrcServer: srcServer, DstServer: dstServer, Dst: dst}
}
