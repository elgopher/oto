package mux

import (
	"runtime"
	"sync"
)

type LivePlayer struct {
	reader       LiveReader
	mux          *Mux
	volume       float64
	reusedBuffer []float32
	err          error
	mutex        sync.Mutex
}

type LiveReader interface {
	// Read is executed directly in the render thread, therefore it must be fast.
	// It should not do any I/O.
	Read(buf []float32) (n int, err error)
}

func (p *LivePlayer) Close() error {
	runtime.SetFinalizer(p, nil)

	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.closeImpl()
}

func (p *LivePlayer) closeImpl() error {
	p.mux.removePlayer(p)
	return nil
}

func (p *LivePlayer) Volume() float64 {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.volume
}

func (p *LivePlayer) SetVolume(volume float64) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.volume = volume
}

func (p *LivePlayer) Err() error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.err
}

func (p *LivePlayer) readBufferAndAdd(buf []float32) int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.reusedBuffer) < len(buf) {
		p.reusedBuffer = make([]float32, len(buf))
	}

	n, err := p.reader.Read(p.reusedBuffer[:len(buf)])
	if err != nil {
		p.err = err
		_ = p.closeImpl()
		return n
	}

	volume := float32(p.volume)

	for i := 0; i < n; i++ {
		buf[i] += p.reusedBuffer[i] * volume
	}

	return n
}

func (p *LivePlayer) canReadSourceToBuffer() bool {
	return false // there is no reading in the background
}

func (p *LivePlayer) readSourceToBuffer() int {
	return 0 // there is no reading in the background
}
