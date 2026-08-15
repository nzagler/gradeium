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
	zero := int16(0)
	if _, err := ValidatePersonalState(PersonalState{Status: StatusCompleted, Rating: &zero}); err != nil {
		t.Fatalf("expected canonical zero rating to be valid: %v", err)
	}
	low := int16(-1)
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

func TestRatingScalePreferenceAndFormatting(t *testing.T) {
	for _, scale := range []string{"1_10", "0_5", "minus5_plus5", "0_100"} {
		if !ValidRatingScale(scale) {
			t.Fatalf("ValidRatingScale(%q) = false", scale)
		}
	}
	if ValidRatingScale("stars") {
		t.Fatal("ValidRatingScale accepted an unsupported value")
	}
	if got := NormalizeRatingScale(""); got != "1_10" {
		t.Fatalf("NormalizeRatingScale(\"\") = %q", got)
	}
	tests := []struct {
		value int16
		scale string
		want  string
	}{
		{0, "1_10", "0.0"},
		{51, "1_10", "5.1"},
		{51, "0_5", "2.55"},
		{50, "minus5_plus5", "+0.0"},
		{49, "minus5_plus5", "-0.1"},
		{100, "0_100", "100"},
	}
	for _, test := range tests {
		got, err := FormatPersonalRating(test.value, test.scale)
		if err != nil || got != test.want {
			t.Errorf("FormatPersonalRating(%d, %q) = (%q, %v), want %q", test.value, test.scale, got, err, test.want)
		}
	}
}
