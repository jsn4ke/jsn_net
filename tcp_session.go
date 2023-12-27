package jsn_net

import (
	"io"
	"net"
	"sync"
	"time"
)

type Pipe interface {
	Post(in any) bool
	Run()
	Close()
}

type (
	TcpCodec interface {
		Read(io.Reader) (any, error)
		Write(io.Writer, any)
	}
)

func NewTcpSession(sid uint64, conn *net.TCPConn, pipe Pipe, codec TcpCodec, readTimeout time.Duration, sendChanSize int) *TcpSession {
	s := new(TcpSession)
	s.sid = sid
	s.conn = conn
	s.pipe = pipe
	s.codec = codec
	s.sendChan = make(chan any, sendChanSize)
	s.readTimeout = readTimeout

	s.tag = LifeTag{}
	s.goWg = sync.WaitGroup{}
	return s
}

type TcpSession struct {
	sid         uint64
	conn        net.Conn
	pipe        Pipe
	codec       TcpCodec
	sendChan    chan any
	readTimeout time.Duration

	tag  LifeTag
	goWg sync.WaitGroup
}

func (s *TcpSession) Post(fn func()) bool {
	if !s.tag.IsRunning() {
		return false
	}
	s.pipe.Post(&TcpEventOutSide{
		S:    s,
		Func: fn,
	})
	return true
}

// 主loop阻塞问题
// 同步处理问题
// tcp 写肯定需要保证不丢失
// todo
func (s *TcpSession) Send(msg any) bool {
	// 如果选择同步写，就阻塞
	// 如果异步，是否阻塞外部写？限流？
	s.sendChan <- msg
	return true
}

func (s *TcpSession) Close() {
	if !s.tag.IsRunning() {
		return
	}
	s.tag.SetRunning(false)
	s.manualClose()
}

func (s *TcpSession) run() {
	if s.tag.IsRunning() {
		return
	}
	s.tag.SetRunning(true)

	s.safeGo("session pipe", s.pipe.Run)
	s.safeGo("session wirte", s.write)

	s.pipe.Post(&TcpEventAdd{
		S: s,
	})
	s.read()
	s.goWg.Wait()
	s.tag.SetRunning(false)
}

func (s *TcpSession) read() {
	for {
		if 0 != s.readTimeout {
			if err := s.conn.SetReadDeadline(time.Now().Add(s.readTimeout)); nil != err {
				s.pipe.Post(&TcpEventClose{
					S:           s,
					Err:         err,
					ManualClose: !s.tag.IsRunning(),
				})
				break
			}
		}
		packet, err := s.codec.Read(s.conn)
		if nil != err {
			s.pipe.Post(&TcpEventClose{
				S:           s,
				Err:         err,
				ManualClose: !s.tag.IsRunning(),
			})
			break
		}
		s.pipe.Post(&TcpEventPacket{
			S:      s,
			Packet: packet,
		})
	}
	s.sendChan <- nil
}

func (s *TcpSession) write() {
	for {
		msg := <-s.sendChan
		if nil == msg {
			break
		}
		s.codec.Write(s.conn, msg)
	}
	if nil != s.conn {
		_ = s.conn.Close()
	}
	s.pipe.Close()
}

func (s *TcpSession) safeGo(name string, fn func()) {
	s.goWg.Add(1)
	go func() {
		defer func() {
			s.goWg.Done()
		}()
		fn()
	}()
}

func (s *TcpSession) manualClose() {
	_ = s.conn.(*net.TCPConn).CloseRead()
	_ = s.conn.SetReadDeadline(time.Now())
}
