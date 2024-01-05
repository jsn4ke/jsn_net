package jsn_rpc

import "github.com/jsn4ke/jsn_net"

var (
	RequestPool  *jsn_net.SyncPool[Request]
	ResponsePool *jsn_net.SyncPool[Response]
	CallPool     *jsn_net.SyncPool[Call]
	AsyncRpcPool *jsn_net.SyncPool[AsyncRpc]
)

func init() {
	RequestPool = new(jsn_net.SyncPool[Request])
	if err := RequestPool.Init(); nil != err {
		panic(err)
	}
	ResponsePool = new(jsn_net.SyncPool[Response])
	if err := ResponsePool.Init(); nil != err {
		panic(err)
	}
	CallPool = new(jsn_net.SyncPool[Call])
	if err := CallPool.Init(); nil != err {
		panic(err)
	}
	AsyncRpcPool = new(jsn_net.SyncPool[AsyncRpc])
	if err := AsyncRpcPool.Init(); nil != err {
		panic(err)
	}
}

type Request struct {
	Seq    uint64
	CmdId  uint32
	Body   []byte
	Oneway bool

	DoneWithError func(uint64, error)
}

func (r *Request) Release() {
	r.Seq = 0
	r.CmdId = 0
	r.Body = nil
	r.Oneway = false

	r.DoneWithError = nil
}

type Response struct {
	Seq   uint64
	Error string
	Body  []byte
}

func (r *Response) Release() {
	r.Seq = 0
	r.Error = ""
	r.Body = nil
}
