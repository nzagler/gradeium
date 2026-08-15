package backups

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
	"time"
)

func TestCodecRoundTripAndStrictStructure(t *testing.T) {
	document := Document{
		Format: Format, Version: FormatVersion, CreatedAt: time.Now().UTC(), ApplicationVersion: "test",
		Users: []User{}, Games: []Game{}, Movies: []Movie{}, TVShows: []TVShow{},
		EpisodeProgress: []Progress{}, Settings: []Setting{},
	}
	var encoded bytes.Buffer
	if err := Encode(&encoded, document); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Format != Format || decoded.Version != FormatVersion || decoded.ApplicationVersion != "test" {
		t.Fatalf("decoded document = %#v", decoded)
	}

	var unknown bytes.Buffer
	writer := gzip.NewWriter(&unknown)
	_, _ = writer.Write([]byte(`{"format":"gradeium-backup","version":1,"createdAt":"2026-08-15T00:00:00Z","applicationVersion":"test","users":[],"games":[],"movies":[],"tvShows":[],"episodeProgress":[],"settings":[],"clientSecret":"must-not-be-accepted"}`))
	_ = writer.Close()
	if _, err := Decode(bytes.NewReader(unknown.Bytes())); err == nil {
		t.Fatal("Decode() accepted an unknown secret-like field")
	}

	withTrailingData := append(append([]byte{}, encoded.Bytes()...), []byte("not-another-backup")...)
	if _, err := Decode(bytes.NewReader(withTrailingData)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("Decode() trailing data error = %v", err)
	}
}

func TestCodecRejectsTruncatedAndOversizedDocuments(t *testing.T) {
	var truncated bytes.Buffer
	writer := gzip.NewWriter(&truncated)
	_, _ = writer.Write([]byte(`{"format":"gradeium-backup"`))
	_ = writer.Close()
	if _, err := Decode(bytes.NewReader(truncated.Bytes())); err == nil {
		t.Fatal("Decode() accepted truncated JSON")
	}

	payload := `{"format":"gradeium-backup","version":1,"createdAt":"2026-08-15T00:00:00Z","applicationVersion":"` + strings.Repeat("x", 4096) + `","users":[],"games":[],"movies":[],"tvShows":[],"episodeProgress":[],"settings":[]}`
	var oversized bytes.Buffer
	writer = gzip.NewWriter(&oversized)
	_, _ = writer.Write([]byte(payload))
	_ = writer.Close()
	if _, err := decodeWithLimit(bytes.NewReader(oversized.Bytes()), 512); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("decodeWithLimit() error = %v, want size limit", err)
	}
}

func TestValidateThemePreferenceAndLegacyDefault(t *testing.T) {
	user := User{
		ID:          "0198b0f1-0000-7000-8000-000000000001",
		Preferences: Preferences{DefaultLibrarySort: "rating_desc", PreferredView: "grid"},
	}
	document := Document{
		Format: Format, Version: FormatVersion, CreatedAt: time.Now().UTC(), ApplicationVersion: "phase5",
		Users: []User{user}, Games: []Game{}, Movies: []Movie{}, TVShows: []TVShow{},
		EpisodeProgress: []Progress{}, Settings: []Setting{},
	}
	if err := Validate(document); err != nil {
		t.Fatalf("Validate() rejected an older backup without appearance or rating-scale preferences: %v", err)
	}
	document.Users[0].Preferences.RatingScale = "stars"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "preferences") {
		t.Fatalf("Validate() invalid rating scale error = %v", err)
	}
	document.Users[0].Preferences.RatingScale = "0_10"
	document.Users[0].Preferences.Theme = "neon"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "preferences") {
		t.Fatalf("Validate() invalid theme error = %v", err)
	}
}

func TestValidateRejectsIdentityAndRatingConflicts(t *testing.T) {
	now := time.Now().UTC()
	userID := "0198b0f1-0000-7000-8000-000000000001"
	gameID := "0198b0f1-0000-7000-8000-000000000002"
	rating := int16(80)
	document := Document{
		Format: Format, Version: FormatVersion, CreatedAt: now, ApplicationVersion: "test",
		Users: []User{{ID: userID, Preferences: Preferences{DefaultLibrarySort: "rating_desc", PreferredView: "grid"}}},
		Games: []Game{{
			ID: gameID, ProviderID: 1, Title: "Test", GameType: "main_game", Genres: []string{}, GameModes: []string{}, Platforms: []string{}, Screenshots: []string{}, ExternalLinks: []ExternalLink{}, Artwork: []Artwork{}, AdditionalContent: []GameContent{}, RelatedReleases: []GameRelationship{},
			MetadataRefreshedAt: now, Users: []PersonalState{{UserID: userID, Status: "backlog", Rating: &rating, DateAdded: now}},
		}},
		Movies: []Movie{}, TVShows: []TVShow{}, EpisodeProgress: []Progress{}, Settings: []Setting{},
	}
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "Backlog") {
		t.Fatalf("Validate() error = %v, want Backlog rating rejection", err)
	}
	document.Games[0].Users[0].Status = "completed"
	document.Users = append(document.Users, document.Users[0])
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "duplicate user") {
		t.Fatalf("Validate() error = %v, want duplicate user rejection", err)
	}
}

func TestServicePathRejectsTraversal(t *testing.T) {
	service := &Service{directory: t.TempDir()}
	if _, err := service.path(`..\master.key`); err == nil {
		t.Fatal("path() accepted traversal")
	}
	filename := "gradeium-20260815T000000.000000000Z-manual.json.gz"
	path, err := service.path(filename)
	if err != nil || !strings.HasSuffix(path, filename) {
		t.Fatalf("path() = (%q, %v)", path, err)
	}
}
