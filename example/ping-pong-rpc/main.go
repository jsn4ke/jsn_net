package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"time"

	"github.com/jsn4ke/jsn_net"
	jsn_rpc "github.com/jsn4ke/jsn_net/rpc"
)

var (
	addr      = flag.String("addr", "", "listen/dial addres")
	clientNum = flag.Int("client-num", 1, "run client num")
	mode      = flag.String("mode", "", "\"client/server\" mode")

	mc []int64
)

const (
	mc_ping = iota
	mc_pong
	mc_success
	mc_failure
	mc_timeout
	mc_cancle
	mc_no_session
	mc_max
)

func count(index int) {
	atomic.AddInt64(&mc[index], 1)
}

func createSnapshot() []int64 {
	var out []int64
	for i := range mc {
		out = append(out, atomic.LoadInt64(&mc[i]))
	}
	return out
}

var _ jsn_rpc.RpcUnit = (*ping)(nil)

type ping struct {
	Data string `json:"data"`
}

// CmdId implements jsn_rpc.RpcUnit.
func (*ping) CmdId() uint32 {
	return 1
}

// Marshal implements jsn_rpc.RpcUnit.
func (p *ping) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// New implements jsn_rpc.RpcUnit.
func (p *ping) New() jsn_rpc.RpcUnit {
	return new(ping)
}

// Unmarshal implements jsn_rpc.RpcUnit.
func (p *ping) Unmarshal(in []byte) error {
	return json.Unmarshal(in, p)
}

var _ jsn_rpc.RpcUnit = (*pong)(nil)

type pong struct {
	Data string `json:"data"`
}

// CmdId implements jsn_rpc.RpcUnit.
func (*pong) CmdId() uint32 {
	return 2
}

// Marshal implements jsn_rpc.RpcUnit.
func (p *pong) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// New implements jsn_rpc.RpcUnit.
func (p *pong) New() jsn_rpc.RpcUnit {
	return new(pong)
}

// Unmarshal implements jsn_rpc.RpcUnit.
func (p *pong) Unmarshal(in []byte) error {
	return json.Unmarshal(in, p)
}

func runServer() {
	svr := jsn_rpc.NewServer(*addr, 128, 2)
	svr.RegisterExecutor(
		new(ping),
		func(request jsn_rpc.RpcUnit) (response jsn_rpc.RpcUnit, err error) {
			in, _ := request.(*ping)
			if nil == in {
				return nil, errors.New("no type")
			}
			count(mc_pong)
			return &pong{
				Data: "pong",
			}, err
		})

	svr.Start()
}

func runClient() {
	cli := jsn_rpc.NewClient(*addr, 128, 2)
	var lastLost int64
	for {
		count(mc_ping)
		cancel := make(chan struct{})
		go func() {
			time.Sleep(time.Duration(time.Now().UnixNano()%10+100) * time.Millisecond)
			close(cancel)
		}()
		err := cli.Call(&ping{
			Data: "ping",
		}, new(pong), cancel, time.Millisecond*200)
		if nil != err {
			count(mc_failure)
			switch err {
			case jsn_rpc.MaualError:
				count(mc_cancle)
			case jsn_rpc.TimeoutError:
				count(mc_timeout)
			case jsn_rpc.NoSesssionError:
				count(mc_no_session)

			}
			switch err {
			case jsn_rpc.NoSesssionError:
				if 0 == lastLost {
					lastLost = time.Now().Unix()
				}
				if 0 != lastLost && time.Now().Unix()-lastLost > int64(time.Second)*2 {
					return
				}
			default:
				lastLost = 0
			}
		} else {
			count(mc_success)

		}
	}

}

func main() {
	flag.Parse()

	mc = make([]int64, mc_max)

	switch *mode {
	case "server":
		go http.ListenAndServe("127.0.0.1:3434", nil)
		runServer()
		go metrics()
	case "client":
		*clientNum = jsn_net.Clip(*clientNum, 1, 30000)
		for i := 0; i < *clientNum; i++ {
			go runClient()
		}
		go metrics()
	default:
		go http.ListenAndServe("127.0.0.1:3434", nil)
		runServer()
		*clientNum = jsn_net.Clip(*clientNum, 1, 30000)
		for i := 0; i < *clientNum; i++ {
			go runClient()

		}
		go metrics()
	}
	select {}
}

func metrics() {
	tk := time.NewTicker(time.Second)
	ss := createSnapshot()
	for {
		select {
		case <-tk.C:
			ns := createSnapshot()
			fmt.Printf("%v qps:%v,success:%v,failure:%v,timeout:%v,manual:%v,no_session:%v || pool:[%v]\n",
				*mode,
				ns[mc_pong]-ss[mc_pong],
				ns[mc_success]-ss[mc_success],
				ns[mc_failure]-ss[mc_failure],
				ns[mc_timeout]-ss[mc_timeout],
				ns[mc_cancle]-ss[mc_cancle],
				ns[mc_no_session]-ss[mc_no_session],
				append([]int64{}, jsn_rpc.RequestPool.Debug(), jsn_rpc.ResponsePool.Debug(), jsn_rpc.CallPool.Debug()),
			)
			ss = ns
		}
	}
}
