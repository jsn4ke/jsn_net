package jsn_rpc

import "github.com/jsn4ke/jsn_net"

var (
	requestPool  *jsn_net.SyncPool[Request]
	responsePool *jsn_net.SyncPool[Response]
	callPool     *jsn_net.SyncPool[Call]
)

func init() {
	requestPool = new(jsn_net.SyncPool[Request])
	if err := requestPool.Init(); nil != err {
		panic(err)
	}
	responsePool = new(jsn_net.SyncPool[Response])
	if err := responsePool.Init(); nil != err {
		panic(err)
	}
	callPool = new(jsn_net.SyncPool[Call])
	if err := callPool.Init(); nil != err {
		panic(err)
	}
}

type Request struct {
	Seq    uint64
	CmdId  uint32
	Body   []byte
	Oneway bool

	DoneWithError func(err error)
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
