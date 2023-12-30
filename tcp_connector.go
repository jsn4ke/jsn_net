package jsn_net

import (
	"net"
	"sync"
	"time"
)

func NewTcpConnector(addr string, reconnection time.Duration,
	providePipe PipeProvider, codec TcpCodec, readTimeout time.Duration, sendChanSize int) *TcpConnector {
	c := new(TcpConnector)
	c.addr = addr
	c.reconnection = reconnection
	c.providePipe = providePipe
	c.codec = codec
	c.readTimeout = readTimeout
	c.sendChanSize = sendChanSize
	c.mtx = sync.Mutex{}
	c.tag = LifeTag{}
	return c
}

type TcpConnector struct {
	addr         string
	reconnection time.Duration

	sessionConstructor

	mtx     sync.Mutex
	session *TcpSession

	tag LifeTag
}

func (c *TcpConnector) Session() *TcpSession {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.session
}

func (c *TcpConnector) Start() *TcpConnector {
	if c.tag.IsRunning() {
		return c
	}
	go c.connect()
	return c
}

func (c *TcpConnector) Close() {
	if !c.tag.IsRunning() {
		return
	}
	if c.tag.IsStopping() {
		return
	}
	c.tag.StartStopping()
	c.mtx.Lock()
	if nil != c.session {
		c.session.Close()
	}
	c.mtx.Unlock()
	c.tag.WaitStopFinished()
}

func (c *TcpConnector) connect() {
	c.tag.SetRunning(true)
	for {
		conn, err := net.Dial("tcp", c.addr)
		if nil == err {
			logger.Debug("[connector] addr %v start session", c.addr)
			c.startSessoin(conn)
		} else {
			logger.Error("[connector] addr %v dial error %v", c.addr, err)
		}
		if c.tag.IsStopping() || 0 == c.reconnection {
			break
		}
		time.Sleep(c.reconnection)
	}
	logger.Error("[connector] addr %v exit connect loop", c.addr)
	c.tag.SetRunning(false)
	c.tag.EndStopping()
}

func (c *TcpConnector) startSessoin(conn net.Conn) {
	session := NewTcpSession(1, conn.(*net.TCPConn), c.providePipe(), c.codec, c.readTimeout, c.sendChanSize)
	c.mtx.Lock()
	c.session = session
	c.mtx.Unlock()
	if c.tag.IsStopping() {
		conn.Close()
	} else {
		session.run()
	}
	c.mtx.Lock()
	c.session = nil
	c.mtx.Unlock()
	logger.Debug("[connector] addr %v session close", c.addr)
}
