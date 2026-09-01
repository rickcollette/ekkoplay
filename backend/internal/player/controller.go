package player

import (
	"context"
	"errors"
	"sync"
	"time"

	"ekkoplayer/internal/db"
	"ekkoplayer/internal/model"
)

type BroadcastFunc func(string, any)

type Controller struct {
	mu        sync.Mutex
	store     *db.Store
	engine    Engine
	maxVolume int
	broadcast BroadcastFunc
	done      chan struct{}
}

func NewController(store *db.Store, engine Engine, maxVolume int, broadcast BroadcastFunc) *Controller {
	c := &Controller{store: store, engine: engine, maxVolume: maxVolume, broadcast: broadcast, done: make(chan struct{})}
	go c.eventLoop()
	go c.checkpointLoop()
	c.restorePaused()
	return c
}

func (c *Controller) restorePaused() {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return
	}
	if p.TrackID > 0 && p.CurrentSong != nil {
		if c.engine.Play(ctx, p.CurrentSong.FilePath) == nil {
			_ = c.engine.Pause(ctx, true)
			if p.PositionMS > 0 {
				_ = c.engine.Seek(ctx, time.Duration(p.PositionMS)*time.Millisecond)
			}
		}
	}
	p.Status = "paused"
	_ = c.store.SavePlayerState(ctx, p)
}
func (c *Controller) checkpointLoop() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-tick.C:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			c.mu.Lock()
			p, e := c.store.PlayerState(ctx)
			if e == nil && p.Status == "playing" {
				if pos, e := c.engine.Position(ctx); e == nil {
					p.PositionMS = pos.Milliseconds()
					_ = c.store.SavePlayerState(ctx, p)
				}
			}
			c.mu.Unlock()
			cancel()
		}
	}
}

func (c *Controller) eventLoop() {
	for ev := range c.engine.Events() {
		if ev.Name == "end-file" && ev.Reason == "eof" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			p, _ := c.store.PlayerState(ctx)
			if p.Repeat == "track" && p.TrackID > 0 {
				_ = c.PlaySong(ctx, p.TrackID)
			} else {
				_ = c.Next(ctx)
			}
			cancel()
		}
		if ev.Name == "engine-exit" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			c.mu.Lock()
			p, e := c.store.PlayerState(ctx)
			if e == nil {
				p.Status = "paused"
				_ = c.store.SavePlayerState(ctx, p)
			}
			c.mu.Unlock()
			c.emit(ctx)
			cancel()
		}
		if ev.Name == "engine-restarted" {
			c.restorePaused()
			c.emit(context.Background())
		}
	}
}
func (c *Controller) emit(ctx context.Context) {
	if c.broadcast == nil {
		return
	}
	if p, e := c.State(ctx); e == nil {
		c.broadcast("player.state", p)
	}
}

func (c *Controller) State(ctx context.Context) (model.PlayerState, error) {
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return p, e
	}
	if p.Status == "playing" {
		if pos, e := c.engine.Position(ctx); e == nil && pos > 0 {
			p.PositionMS = pos.Milliseconds()
		}
	}
	p.UpdatedAt = time.Now()
	return p, nil
}

func (c *Controller) PlaySong(ctx context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.playSongLocked(ctx, id)
}
func (c *Controller) PlaySongs(ctx context.Context, ids []int64, shuffle bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(ids) == 0 {
		return errors.New("playlist is empty")
	}
	if e := c.store.ReplaceQueue(ctx, ids); e != nil {
		return e
	}
	if shuffle {
		if e := c.store.RebuildShuffle(ctx); e != nil {
			return e
		}
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Shuffle, p.QueueIndex = shuffle, 0
	if e = c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	queue, e := c.store.QueueForPlayback(ctx, shuffle)
	if e != nil {
		return e
	}
	if len(queue) == 0 {
		return errors.New("playlist is empty")
	}
	return c.playSongLocked(ctx, queue[0].Song.ID)
}
func (c *Controller) playSongLocked(ctx context.Context, id int64) error {
	s, e := c.store.Song(ctx, id)
	if e != nil {
		return e
	}
	if e := c.engine.Play(ctx, s.FilePath); e != nil {
		return e
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Status = "playing"
	p.TrackID = id
	p.StationID = 0
	p.PositionMS = 0
	p.CurrentSong = &s
	// Directly selecting a song must also reconcile its queue position. Without
	// this, Next and the UI's "Up next" label use whichever index happened to be
	// persisted by the previously playing track.
	p.QueueIndex = -1
	if queue, queueErr := c.store.QueueForPlayback(ctx, p.Shuffle); queueErr == nil {
		for i := range queue {
			if queue[i].Song.ID == id {
				p.QueueIndex = i
				break
			}
		}
	}
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	_ = c.store.RecordPlay(ctx, id)
	go c.emit(context.Background())
	return nil
}

func (c *Controller) PlayRadio(ctx context.Context, id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, e := c.store.RadioByID(ctx, id)
	if e != nil {
		return e
	}
	if e := c.engine.Play(ctx, r.StreamURL); e != nil {
		return e
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Status = "playing"
	p.TrackID = 0
	p.StationID = id
	p.PositionMS = 0
	p.CurrentRadio = &r
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Pause(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	if p.Status == "stopped" {
		// mpv's stop command unloads the source. "Unpausing" after that is a
		// no-op, even though the persisted player state still knows the track.
		// Reload the source before claiming playback has resumed.
		if p.TrackID > 0 {
			song, songErr := c.store.Song(ctx, p.TrackID)
			if songErr != nil {
				return songErr
			}
			if e = c.engine.Play(ctx, song.FilePath); e != nil {
				return e
			}
			p.CurrentSong = &song
		} else if p.StationID > 0 {
			station, stationErr := c.store.RadioByID(ctx, p.StationID)
			if stationErr != nil {
				return stationErr
			}
			if e = c.engine.Play(ctx, station.StreamURL); e != nil {
				return e
			}
			p.CurrentRadio = &station
		} else {
			return errors.New("nothing is selected")
		}
		p.Status = "playing"
		p.PositionMS = 0
		if e = c.store.SavePlayerState(ctx, p); e != nil {
			return e
		}
		go c.emit(context.Background())
		return nil
	}
	paused := p.Status == "playing"
	if e := c.engine.Pause(ctx, paused); e != nil {
		return e
	}
	if paused {
		p.Status = "paused"
	} else {
		p.Status = "playing"
	}
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}

// Recover rebuilds the current source and resumes it at the saved position.
// This is deliberately stronger than toggling pause for a wedged audio engine.
func (c *Controller) Recover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	position := p.PositionMS
	if p.TrackID > 0 {
		song, songErr := c.store.Song(ctx, p.TrackID)
		if songErr != nil {
			return songErr
		}
		if e = c.engine.Play(ctx, song.FilePath); e != nil {
			return e
		}
		if position > 0 {
			_ = c.engine.Seek(ctx, time.Duration(position)*time.Millisecond)
		}
		p.CurrentSong = &song
	} else if p.StationID > 0 {
		station, stationErr := c.store.RadioByID(ctx, p.StationID)
		if stationErr != nil {
			return stationErr
		}
		if e = c.engine.Play(ctx, station.StreamURL); e != nil {
			return e
		}
		p.CurrentRadio = &station
	} else {
		return errors.New("nothing is selected")
	}
	p.Status = "playing"
	if e = c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.engine.Stop(ctx); e != nil {
		return e
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Status = "stopped"
	p.PositionMS = 0
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Seek(ctx context.Context, ms int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ms < 0 {
		ms = 0
	}
	if e := c.engine.Seek(ctx, time.Duration(ms)*time.Millisecond); e != nil {
		return e
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.PositionMS = ms
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Volume(ctx context.Context, v int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v < 0 {
		v = 0
	}
	if v > c.maxVolume {
		v = c.maxVolume
	}
	if e := c.engine.SetVolume(ctx, v); e != nil {
		return e
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Volume = v
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Shuffle(ctx context.Context, v bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Shuffle = v
	if v {
		if e := c.store.RebuildShuffle(ctx); e != nil {
			return e
		}
		p.QueueIndex = 0
	}
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Repeat(ctx context.Context, v string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v != "off" && v != "track" && v != "queue" {
		return errors.New("invalid repeat mode")
	}
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Repeat = v
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}

func (c *Controller) Next(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nextLocked(ctx)
}
func (c *Controller) nextLocked(ctx context.Context) error {
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	q, e := c.store.QueueForPlayback(ctx, p.Shuffle)
	if e != nil {
		return e
	}
	if len(q) == 0 {
		return errors.New("queue is empty")
	}
	idx := p.QueueIndex + 1
	if idx >= len(q) {
		if p.Repeat == "queue" {
			idx = 0
		} else {
			_ = c.engine.Stop(ctx)
			p.Status = "stopped"
			p.PositionMS = 0
			_ = c.store.SavePlayerState(ctx, p)
			go c.emit(context.Background())
			return nil
		}
	}
	p.QueueIndex = idx
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	return c.playSongLocked(ctx, q[idx].Song.ID)
}
func (c *Controller) Previous(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	q, e := c.store.QueueForPlayback(ctx, p.Shuffle)
	if e != nil {
		return e
	}
	if len(q) == 0 {
		return errors.New("queue is empty")
	}
	idx := p.QueueIndex - 1
	if idx < 0 {
		idx = 0
	}
	p.QueueIndex = idx
	if e := c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	return c.playSongLocked(ctx, q[idx].Song.ID)
}
func (c *Controller) Mute(ctx context.Context, v bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, e := c.store.PlayerState(ctx)
	if e != nil {
		return e
	}
	p.Muted = v
	target := p.Volume
	if v {
		target = 0
	}
	if e = c.engine.SetVolume(ctx, target); e != nil {
		return e
	}
	if e = c.store.SavePlayerState(ctx, p); e != nil {
		return e
	}
	go c.emit(context.Background())
	return nil
}
func (c *Controller) Close() error { close(c.done); return c.engine.Close() }
