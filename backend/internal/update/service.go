package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Release struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Available      bool   `json:"available"`
	PublishedAt    string `json:"published_at,omitempty"`
	ReleaseURL     string `json:"release_url,omitempty"`
	Notes          string `json:"notes,omitempty"`
	AssetURL       string `json:"-"`
	ChecksumURL    string `json:"-"`
}

type Status struct {
	State          string `json:"state"`
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version,omitempty"`
	Message        string `json:"message,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

type Service struct {
	repository string
	stateDir   string
	current    string
	client     *http.Client
}

func New(repository, stateDir, current string) *Service {
	return &Service{repository: strings.TrimSpace(repository), stateDir: stateDir, current: current, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Service) Check(ctx context.Context) (Release, error) {
	if s.repository == "" {
		return Release{}, errors.New("update_repository is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+s.repository+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ekkoPlayer/"+s.current)
	res, err := s.client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Release{}, errors.New("no published update release is available")
	}
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub release check failed (%s)", res.Status)
	}
	var body struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Body        string `json:"body"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Release{}, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if latest == "" {
		return Release{}, errors.New("latest release has no version tag")
	}
	name := fmt.Sprintf("ekkoplayer_%s_linux_%s.tar.gz", latest, runtime.GOARCH)
	var asset, checksum string
	for _, a := range body.Assets {
		if a.Name == name {
			asset = a.URL
		}
		if a.Name == name+".sha256" {
			checksum = a.URL
		}
	}
	if asset == "" || checksum == "" {
		return Release{}, fmt.Errorf("release %s has no verified %s bundle", latest, runtime.GOARCH)
	}
	return Release{CurrentVersion: s.current, LatestVersion: latest, Available: compare(latest, s.current) > 0, PublishedAt: body.PublishedAt, ReleaseURL: body.HTMLURL, Notes: body.Body, AssetURL: asset, ChecksumURL: checksum}, nil
}

func (s *Service) Status() Status {
	b, err := os.ReadFile(filepath.Join(s.stateDir, "status.json"))
	if err == nil {
		var status Status
		if json.Unmarshal(b, &status) == nil {
			status.CurrentVersion = s.current
			return status
		}
	}
	return Status{State: "idle", CurrentVersion: s.current, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
}

func (s *Service) Request(ctx context.Context) (Status, error) {
	release, err := s.Check(ctx)
	if err != nil {
		return Status{}, err
	}
	if !release.Available {
		return Status{}, errors.New("the system is already up to date")
	}
	if err := os.MkdirAll(s.stateDir, 0o750); err != nil {
		return Status{}, err
	}
	request := struct{ Version, AssetURL, ChecksumURL string }{release.LatestVersion, release.AssetURL, release.ChecksumURL}
	b, _ := json.Marshal(request)
	tmp := filepath.Join(s.stateDir, fmt.Sprintf("request.%s.tmp", fingerprint(b)))
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return Status{}, err
	}
	if err := os.Rename(tmp, filepath.Join(s.stateDir, "request.json")); err != nil {
		return Status{}, err
	}
	status := Status{State: "requested", CurrentVersion: s.current, TargetVersion: release.LatestVersion, Message: "Update queued", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	return status, nil
}

func fingerprint(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:6]) }

func compare(a, b string) int {
	parse := func(v string) []int {
		v = strings.TrimPrefix(v, "v")
		var out []int
		for _, p := range strings.Split(v, ".") {
			var n int
			fmt.Sscanf(p, "%d", &n)
			out = append(out, n)
		}
		return out
	}
	x, y := parse(a), parse(b)
	for len(x) < len(y) {
		x = append(x, 0)
	}
	for len(y) < len(x) {
		y = append(y, 0)
	}
	for i := range x {
		if x[i] > y[i] {
			return 1
		}
		if x[i] < y[i] {
			return -1
		}
	}
	return 0
}
