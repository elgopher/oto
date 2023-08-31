package main

import (
	"math"
	"time"

	"github.com/ebitengine/oto/v3"
)

const sampleRate = 44800

func main() {
	options := &oto.NewContextOptions{}
	options.SampleRate = sampleRate
	options.ChannelCount = 1
	options.Format = oto.FormatFloat32LE // format is used by normal Player only

	ctx, ready, err := oto.NewContext(options)
	if err != nil {
		panic(err)
	}
	<-ready

	// NewLivePlayer is a new method which creates player executed directly from the render thread
	player := ctx.NewLivePlayer(&synthesizer{})
	defer player.Close()

	time.Sleep(time.Hour)
}

type synthesizer struct {
	pos float64
}

// Read is fast, because it does not do any I/O. Just generates samples on the fly.
func (s *synthesizer) Read(buf []float32) (n int, err error) {
	// generate 440Hz triangle wave
	step := 440.0 / sampleRate

	for i := range buf {
		s.pos += step
		buf[i] = float32(triangleWave(s.pos))
	}

	return len(buf), nil
}

func triangleWave(pos float64) float64 {
	phase := math.Mod(pos, 1)
	value := math.Abs(phase*2-1)*2 - 1

	return value
}
