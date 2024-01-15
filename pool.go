package go_kcp

import (
	"sync"
)

var (
	bufferPool []*sync.Pool
	segPool    *sync.Pool
	timerPool  *sync.Pool
)

func init() {
	bufferPool = make([]*sync.Pool, 12)
	bufferPool[10] = &sync.Pool{New: func() any {
		return make([]byte, IKCP_MTU_DEF)
	}}
	bufferPool[11] = &sync.Pool{New: func() any {
		return make([]byte, 4096)
	}}
	for i := 0; i < 10; i++ {
		bufferPool[i] = &sync.Pool{New: func() any {
			return make([]byte, 2<<i)
		}}
	}
	segPool = &sync.Pool{New: func() any {
		return &ISeg{}
	}}

	timerPool = &sync.Pool{New: func() any {
		return nil
	}}
}

func getSegFromPool() *ISeg {
	s := segPool.Get().(*ISeg)
	if s.sn != 0 {
		panic("dirty seg in use ")
	}
	return s
}

func putSegToPool(s *ISeg) {
	if s == nil {
		return
	}
	data := s.data
	PutBufferToPool(data)
	s.Reset()
	segPool.Put(s)
}

func GetBufferFromPool(size int) []byte {
	if size == 0 {
		return nil
	}
	if size > IKCP_MTU_DEF {
		return bufferPool[11].Get().([]byte)
	}
	if size > 2<<10 {
		return bufferPool[10].Get().([]byte)
	}
	for i := 0; i < 10; i++ {
		if 2<<i >= size {
			return bufferPool[i].Get().([]byte)
		}
	}
	return nil
}

func PutBufferToPool(buffer []byte) {
	if len(buffer) == 0 {
		return
	}
	if len(buffer) > IKCP_MTU_DEF {
		bufferPool[11].Put(buffer)
		return
	}
	if len(buffer) > 2<<10 {
		bufferPool[10].Put(buffer)
		return
	}
	for i := 0; i < 10; i++ {
		if len(buffer) <= 2<<i {
			bufferPool[i].Put(buffer)
			break
		}
	}
}
