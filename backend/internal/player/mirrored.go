package player

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ZoneConfig struct {
	Name              string `json:"name"`
	Device            string `json:"device"`
	Filter            string `json:"filter,omitempty"`
	Channels          string `json:"channels,omitempty"`
	Format            string `json:"format,omitempty"`
	Enabled           bool   `json:"enabled"`
	Primary           bool   `json:"primary"`
	Muted             bool   `json:"muted"`
	Exclusive         bool   `json:"exclusive"`
	VolumeTrim        int    `json:"volume_trim"`
	DelayMS           int    `json:"delay_ms"`
	BufferMS          int    `json:"buffer_ms"`
	SampleRate        int    `json:"sample_rate"`
	DriftCorrectionMS int    `json:"drift_correction_ms"`
}
type ZoneStatus struct {
	ZoneConfig
	Online          bool   `json:"online"`
	Error           string `json:"error,omitempty"`
	EffectiveVolume int    `json:"effective_volume"`
	DriftMS         int64  `json:"drift_ms"`
}
type mirrorZone struct {
	config  ZoneConfig
	engine  *SupervisedMPV
	online  bool
	lastErr string
	drift   int64
}
type MirroredEngine struct {
	mu           sync.RWMutex
	zones        []*mirrorZone
	primary      *mirrorZone
	masterVolume int
	done         chan struct{}
	closeOnce    sync.Once
}

func StartMirroredMPV(binary, socket string, zones []ZoneConfig, volume int) (*MirroredEngine, error) {
	m := &MirroredEngine{masterVolume: volume, done: make(chan struct{})}
	for i, z := range zones {
		if !z.Enabled {
			m.zones = append(m.zones, &mirrorZone{config: z})
			continue
		}
		o := OutputOptions{Name: z.Name, Device: z.Device, Filter: z.Filter, Channels: z.Channels, Format: z.Format, Volume: clampVolume(volume + z.VolumeTrim), DelayMS: z.DelayMS, BufferMS: z.BufferMS, SampleRate: z.SampleRate, Muted: z.Muted, Exclusive: z.Exclusive}
		e, err := StartSupervisedOutput(binary, fmt.Sprintf("%s.%d", socket, i), o)
		mz := &mirrorZone{config: z, engine: e, online: err == nil}
		if err != nil {
			mz.lastErr = err.Error()
		}
		m.zones = append(m.zones, mz)
		if z.Primary {
			m.primary = mz
			if err != nil {
				return nil, fmt.Errorf("primary output %s: %w", z.Name, err)
			}
		}
	}
	if m.primary == nil || m.primary.engine == nil {
		return nil, errors.New("no primary audio output")
	}
	go m.correctDrift()
	return m, nil
}
func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
func (m *MirroredEngine) fanout(ctx context.Context, fn func(*SupervisedMPV) error) error {
	m.mu.RLock()
	zs := append([]*mirrorZone(nil), m.zones...)
	primary := m.primary
	m.mu.RUnlock()
	var wg sync.WaitGroup
	var pe error
	for _, z := range zs {
		if z.engine == nil {
			continue
		}
		wg.Add(1)
		go func(z *mirrorZone) {
			defer wg.Done()
			e := fn(z.engine)
			m.mu.Lock()
			z.online = e == nil
			if e != nil {
				z.lastErr = e.Error()
			} else {
				z.lastErr = ""
			}
			if z == primary {
				pe = e
			}
			m.mu.Unlock()
		}(z)
	}
	wg.Wait()
	return pe
}
func (m *MirroredEngine) Play(c context.Context, p string) error {
	return m.fanout(c, func(e *SupervisedMPV) error { return e.Play(c, p) })
}
func (m *MirroredEngine) Pause(c context.Context, v bool) error {
	return m.fanout(c, func(e *SupervisedMPV) error { return e.Pause(c, v) })
}
func (m *MirroredEngine) Stop(c context.Context) error {
	return m.fanout(c, func(e *SupervisedMPV) error { return e.Stop(c) })
}
func (m *MirroredEngine) Seek(c context.Context, v time.Duration) error {
	return m.fanout(c, func(e *SupervisedMPV) error { return e.Seek(c, v) })
}
func (m *MirroredEngine) SetVolume(c context.Context, v int) error {
	m.mu.Lock()
	m.masterVolume = v
	zs := append([]*mirrorZone(nil), m.zones...)
	m.mu.Unlock()
	return m.fanout(c, func(e *SupervisedMPV) error {
		for _, z := range zs {
			if z.engine == e {
				return e.SetVolume(c, clampVolume(v+z.config.VolumeTrim))
			}
		}
		return nil
	})
}
func (m *MirroredEngine) Position(c context.Context) (time.Duration, error) {
	m.mu.RLock()
	p := m.primary
	m.mu.RUnlock()
	return p.engine.Position(c)
}
func (m *MirroredEngine) Events() <-chan Event { return m.primary.engine.Events() }
func (m *MirroredEngine) Close() error {
	var first error
	m.closeOnce.Do(func() {
		close(m.done)
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, z := range m.zones {
			if z.engine != nil {
				if e := z.engine.Close(); e != nil && first == nil {
					first = e
				}
			}
		}
	})
	return first
}
func (m *MirroredEngine) Statuses() []ZoneStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ZoneStatus, 0, len(m.zones))
	for _, z := range m.zones {
		out = append(out, ZoneStatus{ZoneConfig: z.config, Online: z.online, Error: z.lastErr, EffectiveVolume: clampVolume(m.masterVolume + z.config.VolumeTrim), DriftMS: z.drift})
	}
	return out
}
func (m *MirroredEngine) SetZone(ctx context.Context, name string, volumeTrim *int, muted *bool, delay *int) error {
	m.mu.Lock()
	var z *mirrorZone
	for _, x := range m.zones {
		if x.config.Name == name {
			z = x
			break
		}
	}
	if z == nil {
		m.mu.Unlock()
		return errors.New("audio output not found")
	}
	if volumeTrim != nil {
		z.config.VolumeTrim = *volumeTrim
	}
	if muted != nil {
		z.config.Muted = *muted
	}
	if delay != nil {
		z.config.DelayMS = *delay
	}
	e := z.engine
	master := m.masterVolume
	m.mu.Unlock()
	if e == nil {
		return errors.New("audio output is disabled")
	}
	if volumeTrim != nil {
		if err := e.SetVolume(ctx, clampVolume(master+*volumeTrim)); err != nil {
			return err
		}
	}
	if muted != nil {
		if err := e.SetMute(ctx, *muted); err != nil {
			return err
		}
	}
	if delay != nil {
		return e.SetDelay(ctx, *delay)
	}
	return nil
}
func (m *MirroredEngine) correctDrift() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-t.C:
			m.correctOnce()
		}
	}
}
func (m *MirroredEngine) correctOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.mu.RLock()
	p := m.primary
	zs := append([]*mirrorZone(nil), m.zones...)
	m.mu.RUnlock()
	base, e := p.engine.Position(ctx)
	if e != nil || base <= 0 {
		return
	}
	for _, z := range zs {
		if z == p || z.engine == nil || z.config.DriftCorrectionMS <= 0 {
			continue
		}
		pos, e := z.engine.Position(ctx)
		if e != nil {
			continue
		}
		drift := (pos - base).Milliseconds()
		m.mu.Lock()
		z.drift = drift
		m.mu.Unlock()
		if drift > int64(z.config.DriftCorrectionMS) || drift < -int64(z.config.DriftCorrectionMS) {
			_ = z.engine.Seek(ctx, base)
		}
	}
}
