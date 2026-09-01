package player

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Name   string
	Reason string
}

type Engine interface {
	Play(context.Context, string) error
	Pause(context.Context, bool) error
	Stop(context.Context) error
	Seek(context.Context, time.Duration) error
	SetVolume(context.Context, int) error
	Position(context.Context) (time.Duration, error)
	Events() <-chan Event
	Close() error
}

type NoopEngine struct {
	mu       sync.Mutex
	position time.Duration
	events   chan Event
}

func NewNoopEngine() *NoopEngine { return &NoopEngine{events: make(chan Event, 4)} }
func (n *NoopEngine) Play(context.Context, string) error {
	n.mu.Lock()
	n.position = 0
	n.mu.Unlock()
	return nil
}
func (n *NoopEngine) Pause(context.Context, bool) error { return nil }
func (n *NoopEngine) Stop(context.Context) error {
	n.mu.Lock()
	n.position = 0
	n.mu.Unlock()
	return nil
}
func (n *NoopEngine) Seek(_ context.Context, d time.Duration) error {
	n.mu.Lock()
	n.position = d
	n.mu.Unlock()
	return nil
}
func (n *NoopEngine) SetVolume(context.Context, int) error { return nil }
func (n *NoopEngine) Position(context.Context) (time.Duration, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.position, nil
}
func (n *NoopEngine) Events() <-chan Event { return n.events }
func (n *NoopEngine) Close() error         { close(n.events); return nil }
