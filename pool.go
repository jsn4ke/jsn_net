package jsn_net

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

type SyncPool[B any] struct {
	data *sync.Pool

	debug int64
}

func (s *SyncPool[B]) Init() error {
	if nil == s.convert(new(B)) {
		return fmt.Errorf("%v need release interface", reflect.ValueOf(new(B)).Type().String())
	}
	s.data = &sync.Pool{New: func() any {
		return new(B)
	}}
	return nil
}

func (s *SyncPool[B]) Get() *B {
	atomic.AddInt64(&s.debug, 1)
	return s.data.Get().(*B)
}

type poolRelease interface {
	Release()
}

func (s *SyncPool[B]) Put(b *B) {
	if nil == b {
		return
	}
	poolRelease.Release(s.convert(b))
	s.data.Put(b)
	atomic.AddInt64(&s.debug, -1)
}

func (s *SyncPool[B]) convert(b *B) poolRelease {
	ret, _ := (any(b)).(poolRelease)
	return ret
}

func (s *SyncPool[B]) Debug() int64 {
	return s.debug
}
