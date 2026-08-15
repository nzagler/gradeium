package media

import "testing"

func TestValidatePersonalState(t *testing.T) {
	rating := int16(87)
	reason := "  Great pacing.  "
	state, err := ValidatePersonalState(PersonalState{
		Status: StatusCompleted, Rating: &rating, RatingReason: &reason,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.RatingReason == nil || *state.RatingReason != "Great pacing." {
		t.Fatalf("unexpected reason: %#v", state.RatingReason)
	}

	if _, err := ValidatePersonalState(PersonalState{Status: StatusBacklog, Rating: &rating}); err == nil {
		t.Fatal("expected backlog rating to be rejected")
	}
	low := int16(9)
	if _, err := ValidatePersonalState(PersonalState{Status: StatusCompleted, Rating: &low}); err == nil {
		t.Fatal("expected invalid rating to be rejected")
	}
}

func TestCommunityRating(t *testing.T) {
	if got := CommunityRating(87.45, 100); got == nil || *got != 87 {
		t.Fatalf("unexpected IGDB rating: %v", got)
	}
	if got := CommunityRating(8.75, 10); got == nil || *got != 88 {
		t.Fatalf("unexpected TMDB rating: %v", got)
	}
	if got := CommunityRating(0, 10); got != nil {
		t.Fatalf("expected missing score, got %v", *got)
	}
}

func TestValidateSearchAndProviderID(t *testing.T) {
	if value, err := ValidateSearch("  Foundation  ", 1); err != nil || value != "Foundation" {
		t.Fatalf("ValidateSearch = (%q, %v)", value, err)
	}
	for _, test := range []struct {
		query string
		page  int
	}{{"x", 1}, {"valid", 0}, {"valid", 501}} {
		if _, err := ValidateSearch(test.query, test.page); err == nil {
			t.Fatalf("ValidateSearch(%q, %d) accepted invalid input", test.query, test.page)
		}
	}
	if err := ValidateProviderID(0); err == nil {
		t.Fatal("ValidateProviderID accepted zero")
	}
}

func TestThemePreference(t *testing.T) {
	for _, theme := range []string{"dark", "light", "system"} {
		if !ValidTheme(theme) {
			t.Fatalf("ValidTheme(%q) = false", theme)
		}
	}
	if ValidTheme("provider") {
		t.Fatal("ValidTheme accepted an unsupported value")
	}
	if got := NormalizeTheme(""); got != "dark" {
		t.Fatalf("NormalizeTheme(\"\") = %q, want dark", got)
	}
}
