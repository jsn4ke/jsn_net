package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"time"

	"github.com/jsn4ke/jsn_net"
)

var (
	addr      = flag.String("addr", "", "listen/dial addres")
	clientNum = flag.Int("client-num", 1, "run client num")
	mode      = flag.String("mode", "", "\"client/server\" mode")

	ping int64
	pong int64
)

var _ jsn_net.Pipe = (*asyncPipe)(nil)

func newAsyncPipe(handler func(any)) jsn_net.Pipe {
	p := new(asyncPipe)
	p.ch = make(chan any, 128)
	p.handler = handler
	go p.consumePipe()
	return p
}

type asyncPipe struct {
	ch chan any

	handler func(any)
}

// Close implements jsn_net.Pipe.
func (*asyncPipe) Close() {
}

// Post implements jsn_net.Pipe.
func (p *asyncPipe) Post(in any) bool {
	p.ch <- in

	return true
}

func (p *asyncPipe) consumePipe() {
	for in := range p.ch {
		p.handler(in)
	}
}

// Run implements jsn_net.Pipe.
func (*asyncPipe) Run() {
}

var _ jsn_net.TcpCodec = (*codec)(nil)

type codec struct{}

// Read implements jsn_net.TcpCodec.
func (*codec) Read(r io.Reader) (any, error) {
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
	return string(body), nil
}

// Write implements jsn_net.TcpCodec.
func (*codec) Write(w io.Writer, in any) {
	body := make([]byte, 4+len(in.(string)))
	binary.BigEndian.PutUint32(body, uint32(len(body)))
	copy(body[4:], []byte(in.(string)))
	w.Write(body)
}

func runServer() {
	listener, err := net.Listen("tcp", *addr)
	if nil != err {
		panic(err)
	}
	pipe := newAsyncPipe(func(a any) {
		switch tp := a.(type) {
		case *jsn_net.TcpEventClose:
			fmt.Println("session close")
		case *jsn_net.TcpEventPacket:
			tp.S.Send("pong")
			atomic.AddInt64(&ping, 1)
		}
	})
	acceptor := jsn_net.NewTcpAcceptor(listener.(*net.TCPListener), 1, 1<<15,
		func() jsn_net.Pipe {
			return pipe
		},
		new(codec), 0, 128)

	acceptor.Start()
}

func runClient() {
	pipe := newAsyncPipe(func(a any) {
		switch tp := a.(type) {
		case *jsn_net.TcpEventAdd:
			tp.S.Send("ping")
		case *jsn_net.TcpEventPacket:
			tp.S.Send("ping")
			atomic.AddInt64(&pong, 1)

		}
	})
	connector := jsn_net.NewTcpConnector(*addr, time.Millisecond*500,
		func() jsn_net.Pipe {
			return pipe
		},
		new(codec), 0, 128)
	connector.Start()
}

func metrics(ptr *int64) {
	tk := time.NewTicker(time.Second)
	var (
		last int64
		qps  int64
	)
	for {
		select {
		case <-tk.C:
			var (
				vo = atomic.LoadInt64(ptr)
			)
			qps = vo - last
			last = vo
			fmt.Println(*mode, "qps ", qps)
		}
	}
}

func main() {
	flag.Parse()

	switch *mode {
	case "server":
		go http.ListenAndServe("127.0.0.1:3434", nil)
		runServer()
		go metrics(&ping)
	case "client":
		*clientNum = jsn_net.Clip(*clientNum, 1, 30000)
		for i := 0; i < *clientNum; i++ {
			runClient()
		}
		go metrics(&pong)
	default:
		go http.ListenAndServe("127.0.0.1:3434", nil)
		runServer()
		*clientNum = jsn_net.Clip(*clientNum, 1, 30000)
		for i := 0; i < *clientNum; i++ {
			runClient()

		}
		go metrics(&ping)
	}
	select {}
}
