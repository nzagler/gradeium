package media

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FormatPersonalRating converts Gradeium's canonical 0-100 integer to the
// user's selected presentation scale without changing the stored value.
func FormatPersonalRating(value int16, scale string) (string, error) {
	if value < 0 || value > 100 {
		return "", errors.New("canonical rating is outside 0 to 100")
	}
	switch NormalizeRatingScale(scale) {
	case "1_10":
		return fmt.Sprintf("%.1f", float64(value)/10), nil
	case "0_5":
		return trimDecimal(float64(value) / 20), nil
	case "minus5_plus5":
		display := float64(value)/10 - 5
		if display >= 0 {
			return fmt.Sprintf("+%.1f", display), nil
		}
		return fmt.Sprintf("%.1f", display), nil
	case "0_100":
		return strconv.Itoa(int(value)), nil
	default:
		return "", errors.New("rating scale is not supported")
	}
}

func RatingScaleLabel(scale string) string {
	switch NormalizeRatingScale(scale) {
	case "0_5":
		return "0 to 5"
	case "minus5_plus5":
		return "-5 to +5"
	case "0_100":
		return "0 to 100"
	default:
		return "1 to 10"
	}
}

func trimDecimal(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}
