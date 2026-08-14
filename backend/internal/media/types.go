package media

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusInProgress Status = "in_progress"
	StatusOnHold     Status = "on_hold"
	StatusAbandoned  Status = "abandoned"
	StatusCompleted  Status = "completed"
)

var validStatuses = map[Status]struct{}{
	StatusBacklog: {}, StatusInProgress: {}, StatusOnHold: {},
	StatusAbandoned: {}, StatusCompleted: {},
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	if _, ok := validStatuses[status]; !ok {
		return "", errors.New("choose a valid status")
	}
	return status, nil
}

type PersonalState struct {
	Status       Status  `json:"status"`
	Rating       *int16  `json:"rating,omitempty"`
	RatingReason *string `json:"ratingReason,omitempty"`
}

func ValidatePersonalState(state PersonalState) (PersonalState, error) {
	status, err := ParseStatus(string(state.Status))
	if err != nil {
		return PersonalState{}, err
	}
	state.Status = status
	if state.Rating != nil && (*state.Rating < 10 || *state.Rating > 100) {
		return PersonalState{}, errors.New("rating must be between 1.0 and 10.0 in 0.1 increments")
	}
	if state.RatingReason != nil {
		reason := strings.TrimSpace(*state.RatingReason)
		if reason == "" {
			state.RatingReason = nil
		} else {
			if len([]rune(reason)) > 4000 {
				return PersonalState{}, errors.New("rating reason must be at most 4000 characters")
			}
			state.RatingReason = &reason
		}
	}
	if state.Rating == nil && state.RatingReason != nil {
		return PersonalState{}, errors.New("a rating reason requires a rating")
	}
	if state.Status == StatusBacklog && (state.Rating != nil || state.RatingReason != nil) {
		return PersonalState{}, errors.New("Backlog items cannot be rated")
	}
	return state, nil
}

type SafeError struct {
	Code    string
	Message string
}

func (err *SafeError) Error() string { return err.Message }

func ValidationError(message string) error {
	return &SafeError{Code: "validation_error", Message: message}
}

func ProviderError(provider string) error {
	return &SafeError{
		Code:    provider + "_unavailable",
		Message: fmt.Sprintf("%s could not be reached. Try again.", strings.ToUpper(provider)),
	}
}

func CommunityRating(score float64, scale float64) *int16 {
	if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 || score > scale || scale <= 0 {
		return nil
	}
	normalized := int16(math.Round(score / scale * 100))
	if normalized < 10 {
		normalized = 10
	}
	if normalized > 100 {
		normalized = 100
	}
	return &normalized
}

type Artwork struct {
	ProviderImageID string `json:"providerImageId"`
	Kind            string `json:"kind"`
	Language        string `json:"language,omitempty"`
	ImageURL        string `json:"imageUrl"`
	ThumbnailURL    string `json:"thumbnailUrl"`
	Width           int32  `json:"width,omitempty"`
	Height          int32  `json:"height,omitempty"`
	Preferred       bool   `json:"preferred"`
	Available       bool   `json:"available"`
}

func ValidateArtworkKind(kind string) error {
	switch kind {
	case "poster", "cover", "backdrop", "logo":
		return nil
	default:
		return errors.New("choose a valid artwork type")
	}
}

func ValidateSearch(query string, page int) (string, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return "", errors.New("search query must contain at least 2 characters")
	}
	if page < 1 || page > 500 {
		return "", errors.New("search page is out of range")
	}
	return query, nil
}

func ValidateProviderID(providerID int64) error {
	if providerID <= 0 {
		return errors.New("provider ID must be positive")
	}
	return nil
}
