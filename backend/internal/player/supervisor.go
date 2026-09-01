package player

import (
	"context"
	"errors"
	"sync"
	"time"
)

// SupervisedMPV keeps a usable mpv child behind the Engine interface. Calls made
// during a restart fail quickly; the controller reconciles persisted state when
// engine-restarted is emitted.
type SupervisedMPV struct {
	mu                                       sync.RWMutex
	binary, socket, audioDevice, audioFilter string
	volume                                   int
	options                                  OutputOptions
	current                                  *MPV
	events                                   chan Event
	done                                     chan struct{}
	closeOnce                                sync.Once
}

func StartSupervisedMPV(binary, socket, audioDevice, audioFilter string, volume int) (*SupervisedMPV, error) {
	return StartSupervisedOutput(binary, socket, OutputOptions{Device: audioDevice, Filter: audioFilter, Volume: volume, BufferMS: 100})
}
func StartSupervisedOutput(binary, socket string, o OutputOptions) (*SupervisedMPV, error) {
	m, err := StartMPVOutput(binary, socket, o)
	if err != nil {
		return nil, err
	}
	s := &SupervisedMPV{binary: binary, socket: socket, audioDevice: o.Device, audioFilter: o.Filter, volume: o.Volume, options: o, current: m, events: make(chan Event, 64), done: make(chan struct{})}
	go s.watch(m)
	return s, nil
}
func (s *SupervisedMPV) watch(m *MPV) {
	for {
		select {
		case <-s.done:
			return
		case ev := <-m.Events():
			if ev.Name != "engine-exit" {
				s.publish(ev)
				continue
			}
			s.mu.Lock()
			if s.current == m {
				s.current = nil
			}
			s.mu.Unlock()
			s.publish(ev)
			s.restart()
			return
		}
	}
}
func (s *SupervisedMPV) restart() {
	delay := 250 * time.Millisecond
	for {
		select {
		case <-s.done:
			return
		case <-time.After(delay):
		}
		s.mu.RLock()
		o := s.options
		o.Volume = s.volume
		s.mu.RUnlock()
		m, err := StartMPVOutput(s.binary, s.socket, o)
		if err != nil {
			if delay < 8*time.Second {
				delay *= 2
			}
			continue
		}
		s.mu.Lock()
		s.current = m
		s.mu.Unlock()
		s.publish(Event{Name: "engine-restarted"})
		go s.watch(m)
		return
	}
}
func (s *SupervisedMPV) SetMute(c context.Context, v bool) error {
	s.mu.Lock()
	s.options.Muted = v
	e := s.current
	s.mu.Unlock()
	if e == nil {
		return errors.New("audio engine is restarting")
	}
	return e.SetMute(c, v)
}
func (s *SupervisedMPV) SetDelay(c context.Context, v int) error {
	s.mu.Lock()
	s.options.DelayMS = v
	e := s.current
	s.mu.Unlock()
	if e == nil {
		return errors.New("audio engine is restarting")
	}
	return e.SetDelay(c, v)
}
func (s *SupervisedMPV) publish(ev Event) {
	select {
	case s.events <- ev:
	default:
	}
}
func (s *SupervisedMPV) engine() (*MPV, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return nil, errors.New("audio engine is restarting")
	}
	return s.current, nil
}
func (s *SupervisedMPV) Play(c context.Context, p string) error {
	e, x := s.engine()
	if x != nil {
		return x
	}
	return e.Play(c, p)
}
func (s *SupervisedMPV) Pause(c context.Context, v bool) error {
	e, x := s.engine()
	if x != nil {
		return x
	}
	return e.Pause(c, v)
}
func (s *SupervisedMPV) Stop(c context.Context) error {
	e, x := s.engine()
	if x != nil {
		return x
	}
	return e.Stop(c)
}
func (s *SupervisedMPV) Seek(c context.Context, d time.Duration) error {
	e, x := s.engine()
	if x != nil {
		return x
	}
	return e.Seek(c, d)
}
func (s *SupervisedMPV) SetVolume(c context.Context, v int) error {
	s.mu.Lock()
	s.volume = v
	e := s.current
	s.mu.Unlock()
	if e == nil {
		return errors.New("audio engine is restarting")
	}
	return e.SetVolume(c, v)
}
func (s *SupervisedMPV) Position(c context.Context) (time.Duration, error) {
	e, x := s.engine()
	if x != nil {
		return 0, x
	}
	return e.Position(c)
}
func (s *SupervisedMPV) Healthy(c context.Context) error {
	e, err := s.engine()
	if err != nil {
		return err
	}
	return e.Healthy(c)
}
func (s *SupervisedMPV) Events() <-chan Event { return s.events }
func (s *SupervisedMPV) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		s.mu.Lock()
		if s.current != nil {
			err = s.current.Close()
			s.current = nil
		}
		s.mu.Unlock()
	})
	return err
}
