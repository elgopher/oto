package oto

import "github.com/ebitengine/oto/v3/internal/mux"

type LivePlayer struct {
	player *mux.LivePlayer
}

// Volume returns the current volume in the range of [0, 1].
// The default volume is 1.
func (p *LivePlayer) Volume() float64 {
	return p.player.Volume()
}

// SetVolume sets the current volume in the range of [0, 1].
func (p *LivePlayer) SetVolume(volume float64) {
	p.player.SetVolume(volume)
}

// Err returns an error if this player has an error.
func (p *LivePlayer) Err() error {
	return p.player.Err()
}

func (p *LivePlayer) Close() error {
	return p.player.Close()
}
