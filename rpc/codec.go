package jsn_rpc

import (
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/jsn4ke/jsn_net"
)

var (
	_ jsn_net.TcpCodec = new(codec)
)

type codec struct{}

const (
	maskRpcRequest = 1 + iota
	maskRpcResponse
)

// Mask Rpc request Rpc Response
// Read implements jsn_net.TcpCodec.
func (c *codec) Read(r io.Reader) (any, error) {
	var mask [1]byte
	_, err := io.ReadFull(r, mask[:])
	if nil != err {
		return nil, err
	}

	switch mask[0] {
	case maskRpcRequest:
		// length-seq-oneway-cmd-body
		return c.readRequest(r)
	case maskRpcResponse:
		// length-seq-errormask-body
		return c.readResponse(r)
	default:
		return nil, fmt.Errorf("[%v] read invalid  mask", mask[0])
	}
}

// Write implements jsn_net.TcpCodec.
func (c *codec) Write(w io.Writer, in any) {
	switch tp := in.(type) {
	case *Request:
		// length-seq-oneway-cmd-body
		c.writeRequest(w, tp)
	case *Response:
		// length-seq-errormask-body
		c.writeResponse(w, tp)
	}
}

// oneway=1 不需要返回消息
func (*codec) readRequest(r io.Reader) (any, error) {
	const (
		lengthOccupied = 4
		seqOccupied    = 8
		onewayOccupied = 1
		cmdOccupied    = 4
		lengthCheck    = lengthOccupied + seqOccupied + onewayOccupied + cmdOccupied
	)
	var lengthBody [4]byte
	_, err := io.ReadFull(r, lengthBody[:])
	if nil != err {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBody[:])
	if length < lengthCheck {
		return nil, fmt.Errorf("receive body too short %v need %v",
			length, lengthCheck)
	}
	body := make([]byte, length-lengthOccupied)
	_, err = io.ReadFull(r, body)
	if nil != err {
		return nil, err
	}
	var next int
	req := RequestPool.Get()
	req.Seq = binary.BigEndian.Uint64(body[next:])
	next += seqOccupied
	if 0 != body[next] {
		req.Oneway = true
	}
	next += onewayOccupied
	req.CmdId = binary.BigEndian.Uint32(body[next:])
	next += cmdOccupied
	req.Body = body[next:]

	return req, nil
}

func (*codec) readResponse(r io.Reader) (any, error) {
	const (
		lengthOccupied = 4
		seqOccupied    = 8
		maskOccupied   = 1
		lengthCheck    = lengthOccupied + seqOccupied + maskOccupied
	)
	var lengthBody [4]byte
	_, err := io.ReadFull(r, lengthBody[:])
	if nil != err {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBody[:])
	if length < lengthCheck {
		return nil, fmt.Errorf("receive body too short %v need %v",
			length, lengthCheck)
	}
	body := make([]byte, length-lengthOccupied)
	_, err = io.ReadFull(r, body)
	if nil != err {
		return nil, err
	}
	var next int
	resp := ResponsePool.Get()
	resp.Seq = binary.BigEndian.Uint64(body[next:])
	next += seqOccupied
	mask := body[next]
	next += maskOccupied
	if 0 == mask {
		resp.Body = body[next:]
	} else {
		errBody := body[next:]
		resp.Error = *(*string)(unsafe.Pointer(&errBody))
	}
	return resp, nil
}

func (*codec) writeRequest(w io.Writer, r *Request) {
	defer RequestPool.Put(r)
	err := func() error {
		const (
			readMaskOccupied = 1
			lengthOccupied   = 4
			seqOccupied      = 8
			onewayOccupied   = 1
			cmdOccupied      = 4
		)
		var (
			length = readMaskOccupied + lengthOccupied + seqOccupied + onewayOccupied + cmdOccupied + len(r.Body)
			body   = make([]byte, length)
			next   = 0
		)
		body[next] = maskRpcRequest
		next += readMaskOccupied
		binary.BigEndian.PutUint32(body[next:], uint32(length)-readMaskOccupied)
		next += lengthOccupied
		binary.BigEndian.PutUint64(body[next:], r.Seq)
		next += seqOccupied
		if r.Oneway {
			body[next] = 1
		}
		next += onewayOccupied
		binary.BigEndian.PutUint32(body[next:], r.CmdId)
		next += cmdOccupied
		copy(body[next:], r.Body)
		_, err := w.Write(body)
		return err
	}()
	if nil != err && nil != r.DoneWithError {
		r.DoneWithError(err)
	}
}

func (*codec) writeResponse(w io.Writer, r *Response) {
	defer ResponsePool.Put(r)
	const (
		readMaskOccupied = 1
		lengthOccupied   = 4
		seqOccupied      = 8
		maskOccupied     = 1
	)
	var (
		length = readMaskOccupied + lengthOccupied + seqOccupied + maskOccupied
		data   []byte
		next   = 0
		err    error
		mask   byte
	)
	if 0 != len(r.Error) {
		data = []byte(r.Error)
		mask = 1
	} else {
		data = r.Body
	}
	length += len(data)
	body := make([]byte, length)
	body[next] = maskRpcResponse
	next += readMaskOccupied
	binary.BigEndian.PutUint32(body[next:], uint32(length)-readMaskOccupied)
	next += lengthOccupied
	binary.BigEndian.PutUint64(body[next:], r.Seq)
	next += seqOccupied
	body[next] = mask
	next += maskOccupied
	copy(body[next:], data)
	_, err = w.Write(body)
	if nil != err {
		fmt.Printf(" write err %v\n",
			err)
	}
}
