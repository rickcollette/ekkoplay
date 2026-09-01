package api

import (
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"ekkoplayer/internal/player"
)

func (s *Server) audioOutputs(w http.ResponseWriter, _ *http.Request) {
	if s.audio == nil {
		writeErr(w, 503, errors.New("audio engine is disabled"))
		return
	}
	writeJSON(w, 200, s.audio.Statuses())
}
func (s *Server) updateAudioOutput(w http.ResponseWriter, r *http.Request) {
	if s.audio == nil {
		writeErr(w, 503, errors.New("audio engine is disabled"))
		return
	}
	name, e := url.PathUnescape(r.PathValue("name"))
	if e != nil {
		writeErr(w, 400, e)
		return
	}
	var b struct {
		VolumeTrim        *int    `json:"volume_trim"`
		Muted             *bool   `json:"muted"`
		DelayMS           *int    `json:"delay_ms"`
		Device            *string `json:"device"`
		BufferMS          *int    `json:"buffer_ms"`
		Channels          *string `json:"channels"`
		SampleRate        *int    `json:"sample_rate"`
		Format            *string `json:"format"`
		Exclusive         *bool   `json:"exclusive"`
		Filter            *string `json:"filter"`
		DriftCorrectionMS *int    `json:"drift_correction_ms"`
	}
	if e = decode(r, &b); e != nil {
		writeErr(w, 400, e)
		return
	}
	if b.VolumeTrim != nil && (*b.VolumeTrim < -100 || *b.VolumeTrim > 100) {
		writeErr(w, 400, errors.New("volume_trim must be -100..100"))
		return
	}
	if b.DelayMS != nil && (*b.DelayMS < -5000 || *b.DelayMS > 5000) {
		writeErr(w, 400, errors.New("delay_ms must be -5000..5000"))
		return
	}
	if b.Device != nil && strings.TrimSpace(*b.Device) == "" {
		writeErr(w, 400, errors.New("device is required"))
		return
	}
	if b.BufferMS != nil && (*b.BufferMS < 20 || *b.BufferMS > 5000) {
		writeErr(w, 400, errors.New("buffer_ms must be 20..5000"))
		return
	}
	if b.SampleRate != nil && *b.SampleRate != 0 && (*b.SampleRate < 8000 || *b.SampleRate > 384000) {
		writeErr(w, 400, errors.New("sample_rate must be 8000..384000"))
		return
	}
	if b.DriftCorrectionMS != nil && (*b.DriftCorrectionMS < 1 || *b.DriftCorrectionMS > 1000) {
		writeErr(w, 400, errors.New("drift_correction_ms must be 1..1000"))
		return
	}
	if e = s.audio.SetZone(r.Context(), name, b.VolumeTrim, b.Muted, b.DelayMS); e != nil {
		writeErr(w, 409, e)
		return
	}
	var trim int
	var muted bool
	var delay int
	for _, x := range s.audio.Statuses() {
		if x.Name == name {
			trim = x.VolumeTrim
			muted = x.Muted
			delay = x.DelayMS
		}
	}
	current := player.ZoneStatus{}
	for _, x := range s.audio.Statuses() {
		if x.Name == name {
			current = x
		}
	}
	device, buffer, channels, rate, format, exclusive, filter, drift := current.Device, current.BufferMS, current.Channels, current.SampleRate, current.Format, current.Exclusive, current.Filter, current.DriftCorrectionMS
	if b.Device != nil {
		device = strings.TrimSpace(*b.Device)
	}
	if b.BufferMS != nil {
		buffer = *b.BufferMS
	}
	if b.Channels != nil {
		channels = *b.Channels
	}
	if b.SampleRate != nil {
		rate = *b.SampleRate
	}
	if b.Format != nil {
		format = *b.Format
	}
	if b.Exclusive != nil {
		exclusive = *b.Exclusive
	}
	if b.Filter != nil {
		filter = *b.Filter
	}
	if b.DriftCorrectionMS != nil {
		drift = *b.DriftCorrectionMS
	}
	_, e = s.store.DB.ExecContext(r.Context(), `INSERT INTO audio_output_overrides(name,volume_trim,muted,delay_ms,device,buffer_ms,channels,sample_rate,sample_format,exclusive,audio_filter,drift_correction_ms,configured) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(name) DO UPDATE SET volume_trim=excluded.volume_trim,muted=excluded.muted,delay_ms=excluded.delay_ms,device=excluded.device,buffer_ms=excluded.buffer_ms,channels=excluded.channels,sample_rate=excluded.sample_rate,sample_format=excluded.sample_format,exclusive=excluded.exclusive,audio_filter=excluded.audio_filter,drift_correction_ms=excluded.drift_correction_ms,configured=1,updated_at=CURRENT_TIMESTAMP`, name, trim, muted, delay, device, buffer, channels, rate, format, exclusive, filter, drift)
	if e != nil {
		writeErr(w, 500, e)
		return
	}
	s.hub.Broadcast("audio.outputs", s.audio.Statuses())
	writeJSON(w, 200, s.audio.Statuses())
}
func (s *Server) audioDevices(w http.ResponseWriter, r *http.Request) {
	cmd := exec.CommandContext(r.Context(), s.cfg.MPVBinary, "--no-config", "--audio-device=help")
	raw, e := cmd.CombinedOutput()
	if e != nil {
		writeErr(w, 503, e)
		return
	}
	lines := strings.Split(string(raw), "\n")
	type device struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	out := make([]device, 0)
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "list of") {
			continue
		}
		token := strings.Fields(line)[0]
		id := strings.Trim(token, "'\"")
		relevantALSA := id == "alsa" || strings.HasPrefix(id, "alsa/plughw:") || strings.HasPrefix(id, "alsa/sysdefault:") || strings.HasPrefix(id, "alsa/hdmi:")
		if id != "auto" && !relevantALSA && id != "pipewire" && id != "pulse" {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		label := strings.TrimSpace(strings.TrimPrefix(line, token))
		label = strings.Trim(label, "() ")
		if label == "" {
			label = id
		}
		out = append(out, device{ID: id, Label: label})
	}
	writeJSON(w, 200, map[string]any{"devices": out})
}
