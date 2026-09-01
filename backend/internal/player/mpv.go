package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type MPV struct {
	binary      string
	socket      string
	audioDevice string
	cmd         *exec.Cmd
	events      chan Event
	closeOnce   sync.Once
}

type OutputOptions struct {
	Name, Device, Filter, Channels, Format string
	Volume, DelayMS, BufferMS, SampleRate  int
	Muted, Exclusive                       bool
}

func StartMPV(binary, socket, audioDevice, audioFilter string, volume int) (*MPV, error) {
	return StartMPVOutput(binary, socket, OutputOptions{Device: audioDevice, Filter: audioFilter, Volume: volume, BufferMS: 100})
}
func StartMPVOutput(binary, socket string, o OutputOptions) (*MPV, error) {
	_ = os.Remove(socket)
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, err
	}
	if o.BufferMS < 20 {
		o.BufferMS = 100
	}
	args := []string{"--idle=yes", "--no-terminal", "--really-quiet", "--no-video", "--cache=no", "--audio-buffer=" + strconv.FormatFloat(float64(o.BufferMS)/1000, 'f', 3, 64), "--input-ipc-server=" + socket, "--gapless-audio=yes", "--replaygain=album", "--volume=" + strconv.Itoa(o.Volume), "--mute=" + map[bool]string{true: "yes", false: "no"}[o.Muted], "--audio-delay=" + strconv.FormatFloat(float64(o.DelayMS)/1000, 'f', 3, 64)}
	if o.Filter != "" {
		args = append(args, "--af="+o.Filter)
	}
	if o.Device != "" && o.Device != "auto" {
		args = append(args, "--audio-device="+o.Device)
	}
	if o.Channels != "" {
		args = append(args, "--audio-channels="+o.Channels)
	}
	if o.SampleRate > 0 {
		args = append(args, "--audio-samplerate="+strconv.Itoa(o.SampleRate))
	}
	if o.Format != "" {
		args = append(args, "--audio-format="+o.Format)
	}
	if o.Exclusive {
		args = append(args, "--audio-exclusive=yes")
	}
	cmd := exec.Command(binary, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	m := &MPV{binary: binary, socket: socket, audioDevice: o.Device, cmd: cmd, events: make(chan Event, 32)}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("unix", socket, 100*time.Millisecond); err == nil {
			c.Close()
			go m.listen()
			go func() {
				_ = cmd.Wait()
				select {
				case m.events <- Event{Name: "engine-exit"}:
				default:
				}
			}()
			return m, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return nil, errors.New("mpv IPC socket did not become ready")
}
func (m *MPV) SetMute(ctx context.Context, v bool) error {
	_, e := m.command(ctx, "set_property", "mute", v)
	return e
}
func (m *MPV) SetDelay(ctx context.Context, ms int) error {
	_, e := m.command(ctx, "set_property", "audio-delay", float64(ms)/1000)
	return e
}

func (m *MPV) command(ctx context.Context, args ...any) (any, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", m.socket)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(deadline)
	}
	payload := map[string]any{"command": args}
	if err := json.NewEncoder(c).Encode(payload); err != nil {
		return nil, err
	}
	var resp struct {
		Error string `json:"error"`
		Data  any    `json:"data"`
	}
	if err := json.NewDecoder(c).Decode(&resp); err != nil {
		return nil, err
	}
	if resp.Error != "success" {
		return resp.Data, fmt.Errorf("mpv: %s", resp.Error)
	}
	return resp.Data, nil
}

func (m *MPV) listen() {
	for {
		c, err := net.Dial("unix", m.socket)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		s := bufio.NewScanner(c)
		for s.Scan() {
			var msg map[string]any
			if json.Unmarshal(s.Bytes(), &msg) != nil {
				continue
			}
			if ev, ok := msg["event"].(string); ok {
				reason, _ := msg["reason"].(string)
				select {
				case m.events <- Event{Name: ev, Reason: reason}:
				default:
				}
			}
		}
		c.Close()
		if m.cmd.ProcessState != nil && m.cmd.ProcessState.Exited() {
			return
		}
	}
}

func (m *MPV) Play(ctx context.Context, path string) error {
	if _, e := m.command(ctx, "loadfile", path, "replace"); e != nil {
		return e
	}
	// mpv preserves its pause property across loadfile. A source restored paused
	// after an engine restart would otherwise make every later Play silently
	// load at 0 while the controller reported "playing".
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if _, e := m.command(ctx, "set_property", "pause", false); e == nil {
			return nil
		} else {
			lastErr = e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
		}
	}
	return lastErr
}
func (m *MPV) Pause(ctx context.Context, p bool) error {
	_, e := m.command(ctx, "set_property", "pause", p)
	return e
}
func (m *MPV) Stop(ctx context.Context) error { _, e := m.command(ctx, "stop"); return e }
func (m *MPV) Seek(ctx context.Context, d time.Duration) error {
	_, e := m.command(ctx, "seek", d.Seconds(), "absolute", "exact")
	return e
}
func (m *MPV) SetVolume(ctx context.Context, v int) error {
	_, e := m.command(ctx, "set_property", "volume", v)
	return e
}

// Healthy verifies that mpv's IPC endpoint is responsive. A media command can
// legitimately fail while a file is loading (or while the player is idle), so
// command errors must not be treated as proof that the output process is down.
func (m *MPV) Healthy(ctx context.Context) error {
	_, err := m.command(ctx, "get_property", "mpv-version")
	return err
}
func (m *MPV) Position(ctx context.Context) (time.Duration, error) {
	d, e := m.command(ctx, "get_property", "time-pos")
	if e != nil {
		return 0, e
	}
	f, ok := d.(float64)
	if !ok {
		return 0, nil
	}
	return time.Duration(f * float64(time.Second)), nil
}
func (m *MPV) Events() <-chan Event { return m.events }
func (m *MPV) Close() error {
	var err error
	m.closeOnce.Do(func() {
		if m.cmd != nil && m.cmd.Process != nil {
			err = m.cmd.Process.Kill()
		}
		_ = os.Remove(m.socket)
	})
	return err
}
