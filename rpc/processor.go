package jsn_rpc

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jsn4ke/jsn_net"
)

var _ jsn_net.Pipe = (*clientPipe)(nil)

type (
	RpcUnit interface {
		CmdId() uint32
		Marshal() ([]byte, error)
		Unmarshal([]byte) error
		New() RpcUnit
	}
	RpcExecuctor func(request RpcUnit) (response RpcUnit, err error)
	RpcCell      struct {
		Unit     RpcUnit
		Executor RpcExecuctor
	}
)

type Call struct {
	Seq     uint64
	In      RpcUnit
	Reply   RpcUnit
	Error   error
	Done    chan *Call
	Expired int64
}

func (c *Call) done() {
	select {
	case c.Done <- c:
	default:

	}
}

func (c *Call) Release() {
	c.In = nil
	c.Reply = nil
	c.Error = nil
	c.Expired = 0

	for {
		select {
		case <-c.Done:
		default:
			c.Done = nil
			return
		}
	}
}

type Client struct {

	// socket
	connector *jsn_net.TcpConnector

	// client
	consumeNum   int32
	seqId        uint64
	mtx          sync.Mutex
	pendings     map[uint64]*Call
	responseDone chan struct{}
	responseChan chan *Response

	consumeWg sync.WaitGroup
	tag       jsn_net.LifeTag
	pipe      *clientPipe
}

func NewClient(addr string, responseChanSize int, consumeNum int32) *Client {
	responseChanSize = jsn_net.Clip(responseChanSize, 1, 1024)
	consumeNum = jsn_net.Clip(consumeNum, 1, 4)
	c := new(Client)
	c.consumeWg = sync.WaitGroup{}
	c.pipe = new(clientPipe)
	c.pipe.cli = c
	c.tag = jsn_net.LifeTag{}

	c.consumeNum = consumeNum
	c.pendings = map[uint64]*Call{}
	c.responseDone = make(chan struct{})
	c.responseChan = make(chan *Response, responseChanSize)

	c.connector = jsn_net.NewTcpConnector(addr, time.Second, func() jsn_net.Pipe {
		return c.pipe
	}, new(codec), 0, 128)

	c.start()
	return c
}

func (c *Client) start() {

	if c.tag.IsRunning() {
		return
	}

	c.tag.SetRunning(true)

	c.connector.Start()

	for i := 0; i < int(c.consumeNum); i++ {
		go c.consumeResponse()
	}

	go func() {
		c.consumeWg.Wait()
		c.tag.SetRunning(false)
		c.tag.EndStopping()
	}()
}

// Close implements jsn_net.Pipe.
func (c *Client) Close() {
	if !c.tag.IsRunning() {
		return
	}
	if c.tag.IsStopping() {
		return
	}
	c.tag.StartStopping()
	close(c.responseDone)
	c.mtx.Lock()
	for _, v := range c.pendings {
		v.done()
	}
	c.pendings = map[uint64]*Call{}
	c.mtx.Unlock()
	c.tag.WaitStopFinished()
}

var (
	MaualError      = errors.New("manul cancel")
	TimeoutError    = errors.New("timeout")
	NoSesssionError = errors.New("no session")
)

func (c *Client) Call(in RpcUnit, reply RpcUnit, cancel <-chan struct{}, timeout time.Duration) error {
	call := c.igo(in, reply)

	if timeout == 0 || timeout > time.Minute {
		timeout = time.Minute
	}
	if nil == cancel {
		cancel = make(<-chan struct{})
	}
	var (
		timer = time.NewTimer(timeout)
	)
	defer func() {
		timer.Stop()
		ok := c.removeCall(call.Seq)
		if !ok {
			CallPool.Put(call)
		}
	}()
	select {
	case call := <-call.Done:
		err := call.Error
		return err
	case <-cancel:
		return MaualError
	case <-timer.C:
		return TimeoutError
	}
}

// Ask
// 双向传输，仅关心报错
func (c *Client) Ask(in RpcUnit, cancel <-chan struct{}, timeout time.Duration) error {
	return c.Call(in, nil, cancel, timeout)
}

// Sync
// 单向传输且不等待，不关注结果
func (c *Client) Sync(in RpcUnit) {
	c.mtx.Lock()
	seq := c.genSeq()
	c.mtx.Unlock()
	session := c.connector.Session()
	if nil != session {
		request := RequestPool.Get()
		request.Seq = seq
		request.CmdId = in.CmdId()
		body, err := in.Marshal()
		if nil != err {
			RequestPool.Put(request)
		} else {
			request.Body = body
			request.Oneway = true
			session.Send(request)
		}
	}
}

func (c *Client) igo(in RpcUnit, reply RpcUnit) *Call {
	call := CallPool.Get()
	call.In = in
	call.Reply = reply
	call.Done = make(chan *Call, 1)

	c.mtx.Lock()
	seq := c.genSeq()
	call.Seq = seq
	if nil == c.pendings {
		c.pendings = map[uint64]*Call{}
	}
	c.pendings[seq] = call
	c.mtx.Unlock()
	session := c.connector.Session()
	var err error
	if nil == session {
		err = NoSesssionError
	} else {
		// 必须要在发送协程序列化，保证非跨协程的write concurrence
		body, err := in.Marshal()
		if nil == err {

			request := RequestPool.Get()
			request.Seq = seq
			request.CmdId = in.CmdId()
			request.Body = body
			request.Oneway = false
			request.DoneWithError = c.doneWithError

			session.Send(request)
		}

	}
	if nil != err {
		c.doneWithError(seq, err)
	}
	return call
}

func (c *Client) doneWithError(seq uint64, err error) {
	c.mtx.Lock()
	call := c.pendings[seq]
	delete(c.pendings, seq)
	c.mtx.Unlock()
	if nil != call {
		call.Error = err
		call.done()
	}
}

func (c *Client) removeCall(seq uint64) bool {
	c.mtx.Lock()
	call, ok := c.pendings[seq]
	if ok {
		delete(c.pendings, seq)
		CallPool.Put(call)
	}
	c.mtx.Unlock()
	return ok
}

type clientPipe struct {
	cli *Client
}

// Close implements jsn_net.Pipe.
func (p *clientPipe) Close() {
	p.cli.mtx.Lock()
	for _, v := range p.cli.pendings {
		v.done()
	}
	p.cli.pendings = map[uint64]*Call{}
	p.cli.mtx.Unlock()
}

// Post implements jsn_net.Pipe.
func (p *clientPipe) Post(in any) bool {
	cli := p.cli
	if !cli.tag.IsRunning() {
		return false
	}
	switch tp := in.(type) {
	case *jsn_net.TcpEventPacket:
		cli.responseChan <- tp.Packet.(*Response)
	}
	return true
}

// Run implements jsn_net.Pipe.
func (*clientPipe) Run() {

}

func (c *Client) genSeq() uint64 {
	return atomic.AddUint64(&c.seqId, 1)
}

func (c *Client) consumeResponse() {

	c.consumeWg.Add(1)
	defer c.consumeWg.Done()

	for {
		select {
		case <-c.responseDone:
			return
		case in := <-c.responseChan:
			c.dealResponse(in)
		}
	}
}

func (p *Client) dealResponse(in *Response) {
	defer ResponsePool.Put(in)
	seq := in.Seq
	p.mtx.Lock()
	call := p.pendings[seq]
	delete(p.pendings, seq)
	p.mtx.Unlock()
	if nil == call {
		// todo 可能超时了
		return
	} else if 0 != len(in.Error) {
		call.Error = errors.New(in.Error)
	} else {
		if nil != call.Reply {
			err := call.Reply.Unmarshal(in.Body)
			if nil != err {
				call.Error = fmt.Errorf("rpc %v in %v reply %v not fix",
					seq, call.In.CmdId(), call.Reply.CmdId())
			}
		}
	}
	call.done()
}

type Server struct {
	acceptor *jsn_net.TcpAcceptor

	// server
	consumeNum      int32
	requestChanDone chan struct{}
	requestChan     chan *wrap[Request]

	// outside
	executor map[uint32]*RpcCell

	consumeWg sync.WaitGroup
	tag       jsn_net.LifeTag
	pipe      *serverPipe
}

func NewServer(addr string, requestChanSize int, consumeNum int32) *Server {
	requestChanSize = jsn_net.Clip(requestChanSize, 1, 1024)
	consumeNum = jsn_net.Clip(consumeNum, 1, 4)

	s := new(Server)
	s.consumeWg = sync.WaitGroup{}
	s.tag = jsn_net.LifeTag{}
	s.pipe = new(serverPipe)
	s.pipe.svr = s

	s.consumeNum = consumeNum
	s.requestChanDone = make(chan struct{})
	s.requestChan = make(chan *wrap[Request], requestChanSize)

	listener, err := net.Listen("tcp", addr)
	if nil != err {
		panic(err)
	}

	s.acceptor = jsn_net.NewTcpAcceptor(listener.(*net.TCPListener), 1, 1<<14, func() jsn_net.Pipe {
		return s.pipe
	}, new(codec), 0, 128)

	return s
}

func (s *Server) RegisterExecutor(unit RpcUnit, executor RpcExecuctor) {
	if s.tag.IsRunning() {
		return
	}
	if nil == s.executor {
		s.executor = map[uint32]*RpcCell{}
	}
	s.executor[unit.CmdId()] = &RpcCell{
		Unit:     unit,
		Executor: executor,
	}
}

func (s *Server) Start() {
	if s.tag.IsRunning() {
		return
	}
	s.tag.SetRunning(true)
	s.acceptor.Start()

	for i := 0; i < int(s.consumeNum); i++ {
		go s.consumeRequest()
	}
	go func() {
		s.consumeWg.Wait()
		s.tag.SetRunning(false)
		s.tag.EndStopping()
	}()
}

func (s *Server) Close() {
	if !s.tag.IsRunning() {
		return
	}
	if s.tag.IsStopping() {
		return
	}
	s.tag.StartStopping()
	close(s.requestChanDone)
	s.tag.WaitStopFinished()
}

var _ jsn_net.Pipe = (*serverPipe)(nil)

type serverPipe struct {
	svr *Server
}

// Close implements jsn_net.Pipe.
func (*serverPipe) Close() {

}

// Post implements jsn_net.Pipe.
func (p *serverPipe) Post(in any) bool {
	svr := p.svr
	if !svr.tag.IsRunning() {
		return false
	}
	switch tp := in.(type) {
	case *jsn_net.TcpEventPacket:
		svr.requestChan <- &wrap[Request]{
			S:    tp.S,
			Body: tp.Packet.(*Request),
		}
	}
	return true
}

// Run implements jsn_net.Pipe.
func (*serverPipe) Run() {
}

func (s *Server) consumeRequest() {
	s.consumeWg.Add(1)
	defer s.consumeWg.Done()

	for {
		select {
		case <-s.requestChanDone:
			return
		case in := <-s.requestChan:
			var (
				response = ResponsePool.Get()
				err      error
			)
			err = func(request *Request) error {
				var (
					seq   = request.Seq
					cmdId = request.CmdId
				)
				response.Seq = seq
				exec, ok := s.executor[cmdId]
				if !ok {
					return fmt.Errorf("cmd %v not supported in server", cmdId)
				}
				in := exec.Unit.New()
				err = in.Unmarshal(request.Body)
				if nil != err {
					return err
				}
				reply, err := exec.Executor(in)
				if !request.Oneway {
					if nil != err {
						return err
					}
					if nil != reply {
						body, err := reply.Marshal()
						if nil != err {
							return err
						}
						response.Body = body
					}
				}
				return nil
			}(in.Body)
			if !in.Body.Oneway {
				if nil != err {
					response.Error = err.Error()
				}
				in.S.Send(response)
			} else {
				ResponsePool.Put(response)
			}
			RequestPool.Put(in.Body)
		}
	}
}

type wrap[A any] struct {
	S    *jsn_net.TcpSession
	Body *A
}

func (w *wrap[A]) Release() {
	w.S = nil
	w.Body = nil
}
