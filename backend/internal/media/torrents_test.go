package media

import "testing"

func TestTorrentAudioFileFiltering(t *testing.T) {
	for _, name := range []string{"disc 1/01 Track.FLAC", "album/song.mp3", "audio.OPUS", "song.m4a"} {
		if !audioFile(name) {
			t.Errorf("expected supported audio file %q", name)
		}
	}
	for _, name := range []string{"cover.jpg", "notes.txt", "video.mkv", "fake.mp3.exe"} {
		if audioFile(name) {
			t.Errorf("unexpected supported file %q", name)
		}
	}
}
