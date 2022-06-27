// Copyright 2021 The Oto Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oto

import (
	"io"
	"runtime"
	"sync"
	"time"
)

type players struct {
	players map[*playerImpl]struct{}
	buf     []float32
	cond    *sync.Cond
}

func newPlayers() *players {
	p := &players{
		cond: sync.NewCond(&sync.Mutex{}),
	}
	go p.loop()
	return p
}

func (ps *players) shouldWait() bool {
	for p := range ps.players {
		if p.canReadSourceToBuffer() {
			return false
		}
	}
	return true
}

func (ps *players) wait() {
	ps.cond.L.Lock()
	defer ps.cond.L.Unlock()

	for ps.shouldWait() {
		ps.cond.Wait()
	}
}

func (ps *players) loop() {
	var players []*playerImpl
	for {
		ps.wait()

		ps.cond.L.Lock()
		players = players[:0]
		for p := range ps.players {
			players = append(players, p)
		}
		ps.cond.L.Unlock()

		allZero := true
		for _, p := range players {
			n := p.readSourceToBuffer()
			if n != 0 {
				allZero = false
			}
		}

		// Sleeping is necessary especially on browsers.
		// Sometimes a player continues to read 0 bytes from the source and this loop can be a busy loop in such case.
		if allZero {
			time.Sleep(time.Millisecond)
		}
	}
}

func (ps *players) addPlayer(player *playerImpl) {
	ps.cond.L.Lock()
	defer ps.cond.L.Unlock()

	if ps.players == nil {
		ps.players = map[*playerImpl]struct{}{}
	}
	ps.players[player] = struct{}{}
	ps.cond.Signal()
}

func (ps *players) removePlayer(player *playerImpl) {
	ps.cond.L.Lock()
	defer ps.cond.L.Unlock()

	delete(ps.players, player)
	ps.cond.Signal()
}

func (ps *players) read(buf []float32) {
	ps.cond.L.Lock()
	players := make([]*playerImpl, 0, len(ps.players))
	for p := range ps.players {
		players = append(players, p)
	}
	ps.cond.L.Unlock()

	for i := range buf {
		buf[i] = 0
	}
	for _, p := range players {
		p.readBufferAndAdd(buf)
	}
	ps.cond.Signal()
}

type player struct {
	p *playerImpl
}

type playerImpl struct {
	context    *context
	players    *players
	src        io.Reader
	volume     float64
	err        error
	state      playerState
	tmpbuf     []byte
	buf        []byte
	eof        bool
	bufferSize int

	m sync.Mutex
}

func (c *context) NewPlayer(src io.Reader) Player {
	return newPlayer(c, c.players, src)
}

func newPlayer(context *context, players *players, src io.Reader) *player {
	p := &player{
		p: &playerImpl{
			context:    context,
			players:    players,
			src:        src,
			volume:     1,
			bufferSize: context.defaultBufferSize(),
		},
	}
	runtime.SetFinalizer(p, (*player).Close)
	return p
}

func (p *player) Err() error {
	return p.p.Err()
}

func (p *playerImpl) Err() error {
	p.m.Lock()
	defer p.m.Unlock()

	return p.err
}

func (p *player) Play() {
	p.p.Play()
}

func (p *playerImpl) Play() {
	// Goroutines don't work effiently on Windows. Avoid using them (hajimehoshi/ebiten#1768).
	if runtime.GOOS == "windows" {
		p.m.Lock()
		defer p.m.Unlock()

		p.playImpl()
	} else {
		ch := make(chan struct{})
		go func() {
			p.m.Lock()
			defer p.m.Unlock()

			close(ch)
			p.playImpl()
		}()
		<-ch
	}
}

func (p *player) SetBufferSize(bufferSize int) {
	p.p.setBufferSize(bufferSize)
}

func (p *playerImpl) setBufferSize(bufferSize int) {
	p.m.Lock()
	defer p.m.Unlock()

	orig := p.bufferSize
	p.bufferSize = bufferSize
	if bufferSize == 0 {
		p.bufferSize = p.context.defaultBufferSize()
	}
	if orig != p.bufferSize {
		p.tmpbuf = nil
	}
}

func (p *playerImpl) ensureTmpBuf() []byte {
	if p.tmpbuf == nil {
		p.tmpbuf = make([]byte, p.bufferSize)
	}
	return p.tmpbuf
}

func (p *playerImpl) playImpl() {
	if p.err != nil {
		return
	}
	if p.state != playerPaused {
		return
	}

	if !p.eof {
		buf := p.ensureTmpBuf()
		for len(p.buf) < p.bufferSize {
			n, err := p.src.Read(buf)
			if err != nil && err != io.EOF {
				p.setErrorImpl(err)
				return
			}
			p.buf = append(p.buf, buf[:n]...)
			if err == io.EOF {
				p.eof = true
				break
			}
		}
	}

	if !p.eof || len(p.buf) > 0 {
		p.state = playerPlay
	}

	p.m.Unlock()
	p.players.addPlayer(p)
	p.m.Lock()
}

func (p *player) Pause() {
	p.p.Pause()
}

func (p *playerImpl) Pause() {
	p.m.Lock()
	defer p.m.Unlock()

	if p.state != playerPlay {
		return
	}
	p.state = playerPaused
}

func (p *player) Reset() {
	p.p.Reset()
}

func (p *playerImpl) Reset() {
	p.m.Lock()
	defer p.m.Unlock()
	p.resetImpl()
}

func (p *playerImpl) resetImpl() {
	if p.state == playerClosed {
		return
	}
	p.state = playerPaused
	p.buf = p.buf[:0]
	p.eof = false
}

func (p *player) IsPlaying() bool {
	return p.p.IsPlaying()
}

func (p *playerImpl) IsPlaying() bool {
	p.m.Lock()
	defer p.m.Unlock()
	return p.state == playerPlay
}

func (p *player) Volume() float64 {
	return p.p.Volume()
}

func (p *playerImpl) Volume() float64 {
	p.m.Lock()
	defer p.m.Unlock()
	return p.volume
}

func (p *player) SetVolume(volume float64) {
	p.p.SetVolume(volume)
}

func (p *playerImpl) SetVolume(volume float64) {
	p.m.Lock()
	defer p.m.Unlock()
	p.volume = volume
}

func (p *player) UnplayedBufferSize() int {
	return p.p.UnplayedBufferSize()
}

func (p *playerImpl) UnplayedBufferSize() int {
	p.m.Lock()
	defer p.m.Unlock()
	return len(p.buf)
}

func (p *player) Close() error {
	runtime.SetFinalizer(p, nil)
	return p.p.Close()
}

func (p *playerImpl) Close() error {
	p.m.Lock()
	defer p.m.Unlock()
	return p.closeImpl()
}

func (p *playerImpl) closeImpl() error {
	p.m.Unlock()
	p.players.removePlayer(p)
	p.m.Lock()

	if p.state == playerClosed {
		return p.err
	}
	p.state = playerClosed
	p.buf = nil
	return p.err
}

func (p *playerImpl) readBufferAndAdd(buf []float32) int {
	p.m.Lock()
	defer p.m.Unlock()

	if p.state != playerPlay {
		return 0
	}

	bitDepthInBytes := p.context.bitDepthInBytes
	n := len(p.buf) / bitDepthInBytes
	if n > len(buf) {
		n = len(buf)
	}
	volume := float32(p.volume)
	src := p.buf[:n*bitDepthInBytes]

	for i := 0; i < n; i++ {
		var v float32
		switch bitDepthInBytes {
		case 1:
			v8 := src[i]
			v = float32(v8-(1<<7)) / (1 << 7)
		case 2:
			v16 := int16(src[2*i]) | (int16(src[2*i+1]) << 8)
			v = float32(v16) / (1 << 15)
		}
		buf[i] += v * volume
	}

	copy(p.buf, p.buf[n*bitDepthInBytes:])
	p.buf = p.buf[:len(p.buf)-n*bitDepthInBytes]

	if p.eof && len(p.buf) == 0 {
		p.state = playerPaused
	}

	return n
}

func (p *playerImpl) canReadSourceToBuffer() bool {
	p.m.Lock()
	defer p.m.Unlock()

	if p.eof {
		return false
	}
	return len(p.buf) < p.bufferSize
}

func (p *playerImpl) readSourceToBuffer() int {
	p.m.Lock()
	defer p.m.Unlock()

	if p.err != nil {
		return 0
	}
	if p.state == playerClosed {
		return 0
	}

	if len(p.buf) >= p.bufferSize {
		return 0
	}

	buf := p.ensureTmpBuf()
	n, err := p.src.Read(buf)

	if err != nil && err != io.EOF {
		p.setErrorImpl(err)
		return 0
	}

	p.buf = append(p.buf, buf[:n]...)
	if err == io.EOF {
		p.eof = true
		if len(p.buf) == 0 {
			p.state = playerPaused
		}
	}
	return n
}

func (p *playerImpl) setErrorImpl(err error) {
	p.err = err
	p.closeImpl()
}
