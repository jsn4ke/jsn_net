package jsn_net

import (
	"sync"
	"sync/atomic"
)

type LifeTag struct {
	running int64

	stoppingWg sync.WaitGroup
	stopping   int64
}

func (t *LifeTag) IsRunning() bool {
	return atomic.LoadInt64(&t.running) != 0
}

func (t *LifeTag) SetRunning(v bool) {
	if v {
		atomic.StoreInt64(&t.running, 1)
	} else {
		atomic.StoreInt64(&t.running, 0)
	}
}

func (t *LifeTag) WaitStopFinished() {
	t.stoppingWg.Wait()
}

func (t *LifeTag) IsStopping() bool {
	return atomic.LoadInt64(&t.stopping) != 0
}

func (t *LifeTag) StartStopping() {
	t.stoppingWg.Add(1)
	atomic.StoreInt64(&t.stopping, 1)
}

func (t *LifeTag) EndStopping() {
	if t.IsStopping() {
		t.stoppingWg.Done()
		atomic.StoreInt64(&t.stopping, 0)
	}
}
