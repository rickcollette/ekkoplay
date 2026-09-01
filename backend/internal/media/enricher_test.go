package media

import (
	"testing"

	"ekkoplayer/internal/model"
)

func recording(title, artist string, duration int64, score int, album string) mbRecording {
	return mbRecording{Title: title, Length: duration, Score: score, ArtistCredit: []artistCredit{{Name: artist}}, Releases: []mbRecordingRelease{{Title: album, Status: "Official", ReleaseGroup: mbReleaseGroup{ID: "group", Title: album, PrimaryType: "Album"}}}}
}

func TestBestRecordingRequiresIndependentFacts(t *testing.T) {
	song := model.Song{Title: "All This Love", Artist: "Unknown Artist", DurationMS: 352653}
	wrong := recording("All This Love", "DeBarge", 200000, 100, "All This Love")
	match := recording("All This Love", "DeBarge", 352000, 85, "All This Love")
	got, _, ok := bestRecording(song, "", "All This Love", []mbRecording{wrong, match})
	if !ok || got.Length != match.Length {
		t.Fatalf("expected title+duration match, got %#v, %v", got, ok)
	}
	if _, _, ok := bestRecording(song, "", "All This Love", []mbRecording{wrong}); ok {
		t.Fatal("accepted title-only match")
	}
}

func TestParserCleansVideoNoiseAndReversedCredits(t *testing.T) {
	a, title := parseName(model.Song{OriginalFilename: "Earth, Wind & Fire - September (Official HD Video).mp3"})
	if a != "Earth, Wind & Fire" || title != "September" {
		t.Fatalf("got %q / %q", a, title)
	}
	a, title = parseName(model.Song{OriginalFilename: "End of Time Lyrics - Beyonce.mp3"})
	if a != "Beyonce" || title != "End of Time" {
		t.Fatalf("got %q / %q", a, title)
	}
}

func TestReleaseSelectionRejectsCompilation(t *testing.T) {
	compilation := mbRecordingRelease{Status: "Official", ReleaseGroup: mbReleaseGroup{ID: "c", Title: "Huge Hits", PrimaryType: "Album", SecondaryTypes: []string{"Compilation"}}}
	original := mbRecordingRelease{Status: "Official", ReleaseGroup: mbReleaseGroup{ID: "o", Title: "Original Album", PrimaryType: "Album", FirstReleaseDate: "1982"}}
	if got := chooseRelease(model.Song{}, []mbRecordingRelease{compilation, original}); got.ReleaseGroup.ID != "o" {
		t.Fatalf("selected %q", got.ReleaseGroup.Title)
	}
}
