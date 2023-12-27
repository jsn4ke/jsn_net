package jsn_net

type (
	TcpEventAdd struct {
		S *TcpSession
	}
	TcpEventClose struct {
		S           *TcpSession
		Err         error
		ManualClose bool
	}
	TcpEventPacket struct {
		S      *TcpSession
		Packet any
	}
	TcpEventOutSide struct {
		S    *TcpSession
		Func func()
	}
)
