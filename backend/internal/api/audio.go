package api

import (
	"errors"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
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
		VolumeTrim *int  `json:"volume_trim"`
		Muted      *bool `json:"muted"`
		DelayMS    *int  `json:"delay_ms"`
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
	_, e = s.store.DB.ExecContext(r.Context(), `INSERT INTO audio_output_overrides(name,volume_trim,muted,delay_ms) VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET volume_trim=excluded.volume_trim,muted=excluded.muted,delay_ms=excluded.delay_ms,updated_at=CURRENT_TIMESTAMP`, name, trim, muted, delay)
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
	out := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(strings.ToLower(line), "list of") {
			out = append(out, line)
		}
	}
	writeJSON(w, 200, map[string]any{"devices": out})
}
