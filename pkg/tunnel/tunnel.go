package tunnel

type Tunnel struct {
	// Tunnel's entrance. Address where new connections are openned.
	Src string
	// Address of the server that listens directly to Src.
	SrcServer string
	// Address of the server that connects directly to Dst.
	DstServer string
	// Tunnel's exit. Address to where the data will go.
	Dst string
}

func New(src, srcServer, dstServer, dst string) Tunnel {
	return Tunnel{Src: src, SrcServer: srcServer, DstServer: dstServer, Dst: dst}
}
