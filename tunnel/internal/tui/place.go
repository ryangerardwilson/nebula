package tui

import (
	"sync"
	"time"
)

// placer writes the session place off the screen loop, latest wins. One
// goroutine writes. set only records the newest place, and a place equal
// to the last one written is skipped.
type placer struct {
	write   func(cursor string) error
	mu      sync.Mutex
	want    string
	written string
	busy    bool
	wake    chan struct{}
	done    chan struct{}
	exited  chan struct{}
}

// placeWait bounds how long stop waits for a write already running, so a
// stuck c3po cannot hold the quit. The session closes after it.
const placeWait = 5 * time.Second

// startPlacer starts the writer. written is the place the session already
// holds, so it is not written again.
func startPlacer(write func(cursor string) error, written string) *placer {
	if write == nil {
		write = func(string) error { return nil }
	}
	p := &placer{
		write:   write,
		want:    written,
		written: written,
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
	}
	go p.run()
	return p
}

// set records place as the newest one. It never waits.
func (p *placer) set(place string) {
	p.mu.Lock()
	p.want = place
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// pending is 1 while a place waits or is being written, else 0.
func (p *placer) pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy || p.want != p.written {
		return 1
	}
	return 0
}

// stop writes the newest place, if it is not written yet, and returns once
// the writer has exited or placeWait has passed, so the session's close
// comes after its last place.
func (p *placer) stop() {
	close(p.done)
	select {
	case <-p.exited:
	case <-time.After(placeWait):
	}
}

func (p *placer) run() {
	defer close(p.exited)
	for {
		select {
		case <-p.done:
			for p.writeNext() {
			}
			return
		case <-p.wake:
		}
		for p.writeNext() {
		}
	}
}

// writeNext writes the newest place unless it was the last one written,
// and reports whether it wrote. A failed write still counts as written, so
// a broken c3po is not retried in a loop.
func (p *placer) writeNext() bool {
	p.mu.Lock()
	place := p.want
	if place == p.written {
		p.mu.Unlock()
		return false
	}
	p.busy = true
	p.mu.Unlock()
	_ = p.write(place)
	p.mu.Lock()
	p.written, p.busy = place, false
	p.mu.Unlock()
	return true
}
