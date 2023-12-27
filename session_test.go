package jsn_net

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"testing"
	"time"
)

var (
	addr = "127.0.0.1:12345"
)

func TestRun(t *testing.T) {

	go http.ListenAndServe("127.0.0.1:12333", nil)

	s := startServer()
	time.Sleep(time.Second)
	go func() {
		<-time.After(time.Second * 100)
		s.Close()
	}()

	var last int32

	go func() {
		for {
			select {
			case <-time.After(time.Millisecond * 500):
				var (
					ping = atomic.LoadInt32(&ping)
					pong = atomic.LoadInt32(&pong)
				)
				fmt.Printf("session num %v ping %v pong %v qps %v\n",
					atomic.LoadInt32(&testSessionNum),
					ping, pong, pong-last)
				last = pong
			}
		}
	}()

	for i := 0; i < 1000; i++ {
		time.Sleep(time.Millisecond * 0)
		sessionRun()
	}

	for {

	}
}

type testPipe struct {
	ch     chan any
	server bool
	done   chan struct{}
}

// Close implements Pipe.
func (p *testPipe) Close() {
	select {
	case p.done <- struct{}{}:
	default:
	}
}

// Post implements Pipe.
func (p *testPipe) Post(in any) bool {
	p.ch <- in
	return true
}

// Run implements Pipe.
func (p *testPipe) Run() {
	defer func() {
		for {
			select {
			case <-p.ch:
			default:
				return
			}
		}
	}()
	for {
		select {
		case <-p.done:
			return
		case in := <-p.ch:
			testHandle(in)
		}
	}
}

var (
	testSessionNum int32
	ping           int32
	pong           int32
)

func testHandle(in any) {

	switch tp := in.(type) {
	case *TcpEventAdd:
		atomic.AddInt32(&testSessionNum, 1)
	case *TcpEventClose:
		fmt.Println("session", tp.S.conn.RemoteAddr().String(), "err %v", tp.Err.Error())
		atomic.AddInt32(&testSessionNum, -1)
	case *TcpEventPacket:
		tp.S.Send(tp.Packet.(*testPacket).data + "recieve")
	case *TcpEventOutSide:
		tp.Func()
	}
}

type testCodec struct {
}

type testPacket struct {
	data string
}

// Read implements TcpCodec.
func (*testCodec) Read(r io.Reader) (any, error) {
	var lengthBody [4]byte
	_, err := io.ReadFull(r, lengthBody[:])
	if nil != err {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBody[:])
	body := make([]byte, length-4)
	_, err = io.ReadFull(r, body)
	if nil != err {
		return nil, err
	}
	return &testPacket{
		data: string(body),
	}, nil
}

// Write implements TcpCodec.
func (*testCodec) Write(w io.Writer, in any) {
	var data []byte
	switch tp := in.(type) {
	case string:
		data = []byte(tp)
	default:
		panic("write invalid type")
	}
	body := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(body, uint32(len(body)))
	copy(body[4:], data)
	w.Write(body)
}

func startServer() *TcpAcceptor {
	listener, err := net.Listen("tcp", addr)
	if nil != err {
		panic(err)
	}
	tcpListener, _ := listener.(*net.TCPListener)
	if nil == tcpListener {
		panic("not tcp listener")
	}
	acceptor := NewTcpAcceptor(tcpListener, 2, 40000, func() Pipe {
		p := new(testPipe)
		p.ch = make(chan any, 128)
		p.done = make(chan struct{}, 1)
		return p
	}, new(testCodec), 0, 64)
	go acceptor.Start()
	return acceptor
}

type testSyncPipe struct{}

// Close implements Pipe.
func (*testSyncPipe) Close() {
}

// Post implements Pipe.
func (*testSyncPipe) Post(in any) bool {
	testHandleSync(in)
	return true
}

// Run implements Pipe.
func (*testSyncPipe) Run() {
}

func testHandleSync(in any) {
	switch tp := in.(type) {
	case *TcpEventAdd:
		atomic.AddInt32(&ping, 1)
		tp.S.Send("hello world")
	case *TcpEventPacket:
		// fmt.Println(tp.Packet.(*testPacket).data)
		atomic.AddInt32(&pong, 1)

		atomic.AddInt32(&ping, 1)
		tp.S.Send("hello world")
	case *TcpEventOutSide:
		tp.Func()
	}
}

func sessionRun() *TcpSession {
	conn, err := net.Dial("tcp", addr)
	if nil != err {
		fmt.Println(err)
		return nil
	}

	session := NewTcpSession(1, conn.(*net.TCPConn), new(testSyncPipe), new(testCodec), 0, 10)
	go func() {
		session.run()
		// time.Sleep(time.Microsecond * 50)
		// tm := time.NewTicker(time.Second)
		// for session.tag.IsRunning() {
		// 	select {
		// 	case <-tm.C:
		// 		session.Send("hello world")
		// 		atomic.AddInt32(&ping, 1)
		// 	}
		// }
	}()
	return session
}
