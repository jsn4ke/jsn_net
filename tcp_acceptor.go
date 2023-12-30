package jsn_net

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func NewTcpAcceptor(listener *net.TCPListener, acceptNum int32, limitConnSize int32,
	providePipe PipeProvider, codec TcpCodec, readTimeout time.Duration, sendChanSize int) *TcpAcceptor {
	// acceptor need
	a := new(TcpAcceptor)
	a.acceptNum = Clip(acceptNum, 1, acceptRunningLimit)
	a.listener = listener
	a.limitConnSize = limitConnSize

	// session need
	a.providePipe = providePipe
	a.codec = codec
	a.readTimeout = readTimeout
	a.sendChanSize = sendChanSize

	// self init
	a.sessions = sync.Map{}
	a.connSize = 0
	a.tag = LifeTag{}
	a.goWg = sync.WaitGroup{}

	return a
}

type TcpAcceptor struct {
	acceptNum     int32
	listener      net.Listener
	limitConnSize int32

	sessionConstructor

	sid      uint64
	sessions sync.Map
	connSize int32
	tag      LifeTag
	goWg     sync.WaitGroup
}

func (a *TcpAcceptor) Close() {
	if !a.tag.IsRunning() {
		return
	}
	if a.tag.IsStopping() {
		return
	}
	a.tag.StartStopping()
	a.listener.Close()
	a.sessions.Range(func(key, value any) bool {
		session, ok := value.(*TcpSession)
		if !ok {
			return true
		}
		session.Close()
		a.sessions.Delete(key)
		return true
	})
	a.tag.WaitStopFinished()
}

func (a *TcpAcceptor) Start() {
	if a.tag.IsRunning() {
		return
	}
	a.tag.SetRunning(true)
	for i := 0; i < int(a.acceptNum); i++ {
		WaitGo(&a.goWg, a.accept)
	}
	go func() {
		a.goWg.Wait()
		a.tag.SetRunning(false)
		a.tag.EndStopping()
	}()

}

func (a *TcpAcceptor) accept() {
	for {
		conn, err := a.listener.Accept()
		if a.tag.IsStopping() {
			if nil != conn {
				conn.Close()
			}
			break
		}
		if nil != err {
			continue
		}
		size := atomic.LoadInt32(&a.connSize)
		if size >= a.limitConnSize {
			conn.Close()
			continue
		}
		atomic.AddInt32(&a.connSize, 1)
		WaitGo(&a.goWg, func() {
			a.runSession(conn)
		})
	}
}

func (a *TcpAcceptor) runSession(conn net.Conn) {
	sid := atomic.AddUint64(&a.sid, 1)
	session := NewTcpSession(sid, conn.(*net.TCPConn), a.providePipe(), a.codec, a.readTimeout, a.sendChanSize)
	if _, ok := a.sessions.Load(sid); ok {
		panic("duplicate key sid")
	}
	a.sessions.Store(sid, session)
	session.run()
	atomic.AddInt32(&a.connSize, -1)
	a.sessions.Delete(sid)
}
