package backups

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nzagler/gradeium/backend/internal/media"
)

const (
	MaxCompressedSize   = 32 << 20
	MaxDecompressedSize = 128 << 20
	maxUsers            = 10_000
	maxMediaItems       = 100_000
	maxEpisodes         = 1_000_000
	maxArtwork          = 1_000_000
)

func Encode(writer io.Writer, document Document) error {
	gzipWriter, err := gzip.NewWriterLevel(writer, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}
	encoder := json.NewEncoder(gzipWriter)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(document); err != nil {
		_ = gzipWriter.Close()
		return fmt.Errorf("encode backup document: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("finish gzip backup: %w", err)
	}
	return nil
}

func Decode(reader io.Reader) (Document, error) {
	return decodeWithLimit(reader, MaxDecompressedSize)
}

func decodeWithLimit(reader io.Reader, maximum int64) (Document, error) {
	buffered := bufio.NewReader(reader)
	gzipReader, err := gzip.NewReader(buffered)
	if err != nil {
		return Document{}, errors.New("backup is not a valid gzip file")
	}
	gzipReader.Multistream(false)
	limited := &io.LimitedReader{R: gzipReader, N: maximum + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		_ = gzipReader.Close()
		if limited.N <= 0 {
			return Document{}, errors.New("backup exceeds the decompressed size limit")
		}
		return Document{}, errors.New("backup contains invalid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		_ = gzipReader.Close()
		return Document{}, errors.New("backup must contain exactly one JSON document")
	}
	if limited.N <= 0 {
		_ = gzipReader.Close()
		return Document{}, errors.New("backup exceeds the decompressed size limit")
	}
	if err := gzipReader.Close(); err != nil {
		return Document{}, errors.New("backup gzip checksum is invalid")
	}
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("backup contains trailing compressed data")
	}
	if err := Validate(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func Validate(document Document) error {
	if document.Format != Format {
		return errors.New("backup format is not supported")
	}
	if document.Version != FormatVersion {
		return fmt.Errorf("backup version %d is not supported", document.Version)
	}
	if document.CreatedAt.IsZero() {
		return errors.New("backup creation time is missing")
	}
	if err := textValue("application version", document.ApplicationVersion, 1, 100); err != nil {
		return err
	}
	if len(document.Users) > maxUsers || len(document.Games)+len(document.Movies)+len(document.TVShows) > maxMediaItems {
		return errors.New("backup contains too many records")
	}

	users := make(map[string]struct{}, len(document.Users))
	identities := make(map[string]struct{}, len(document.Users))
	for _, user := range document.Users {
		if err := uuidv7("user ID", user.ID); err != nil {
			return err
		}
		if _, duplicate := users[user.ID]; duplicate {
			return errors.New("backup contains a duplicate user ID")
		}
		users[user.ID] = struct{}{}
		if (user.OIDCIssuer == nil) != (user.OIDCSubject == nil) {
			return errors.New("backup user OIDC identity is incomplete")
		}
		if user.OIDCIssuer != nil {
			if err := secureURL("OIDC issuer", *user.OIDCIssuer, true); err != nil {
				return err
			}
			if err := textValue("OIDC subject", *user.OIDCSubject, 1, 512); err != nil {
				return err
			}
			identity := *user.OIDCIssuer + "\x00" + *user.OIDCSubject
			if _, duplicate := identities[identity]; duplicate {
				return errors.New("backup contains a duplicate OIDC identity")
			}
			identities[identity] = struct{}{}
		}
		if err := optionalText("display name", user.DisplayName, 500); err != nil {
			return err
		}
		if err := optionalText("email", user.Email, 500); err != nil {
			return err
		}
		if !validSort(user.Preferences.DefaultLibrarySort) || (user.Preferences.PreferredView != "grid" && user.Preferences.PreferredView != "list") || !media.ValidTheme(media.NormalizeTheme(user.Preferences.Theme)) || !media.ValidRatingScale(media.NormalizeRatingScale(user.Preferences.RatingScale)) {
			return errors.New("backup contains invalid Library preferences")
		}
	}

	entityIDs := make(map[string]string)
	gameProviders := make(map[int64]struct{})
	movieProviders := make(map[int64]struct{})
	tvProviders := make(map[int64]struct{})
	verifiedTMDB := make(map[int64]struct{})
	episodeIDs := make(map[string]string)
	seasonIDs := make(map[string]struct{})
	seasonProviderIDs := make(map[int64]struct{})
	episodeProviderIDs := make(map[int64]struct{})
	progressEligible := make(map[string]map[string]struct{})
	artworkCount, episodeCount := 0, 0

	for index := range document.Games {
		item := &document.Games[index]
		if err := validateEntity(item.ID, "game", item.ProviderID, item.Title, entityIDs, gameProviders); err != nil {
			return err
		}
		if err := validateStates(item.Users, users, item.ID, progressEligible); err != nil {
			return err
		}
		if err := validateArtwork(item.ID, "igdb", item.Artwork); err != nil {
			return err
		}
		if err := validatePins(item.Users, item.Artwork, true); err != nil {
			return err
		}
		artworkCount += len(item.Artwork)
		if err := textValue("game type", item.GameType, 1, 200); err != nil {
			return err
		}
		if err := validateCanonicalMetadata(item.ReleaseYear, item.CommunityRating, item.CommunityRatingCount, item.MetadataRefreshedAt); err != nil {
			return fmt.Errorf("backup contains invalid game metadata: %w", err)
		}
		for _, link := range item.ExternalLinks {
			if err := textValue("external-link label", link.Label, 1, 100); err != nil {
				return err
			}
			if err := secureURL("external link", link.URL, false); err != nil {
				return err
			}
		}
		for _, screenshot := range item.Screenshots {
			if err := secureURL("game screenshot", screenshot, false); err != nil {
				return err
			}
		}
		contentIDs := make(map[int64]struct{}, len(item.AdditionalContent))
		for _, content := range item.AdditionalContent {
			if content.ProviderID <= 0 || content.Type == "" {
				return errors.New("backup contains invalid game additional content")
			}
			if _, duplicate := contentIDs[content.ProviderID]; duplicate {
				return errors.New("backup contains duplicate game additional content")
			}
			contentIDs[content.ProviderID] = struct{}{}
			if err := textValue("game additional-content title", content.Title, 1, 500); err != nil {
				return err
			}
			if err := optionalSecureURL("game additional-content cover", content.CoverURL); err != nil {
				return err
			}
		}
		relationships := make(map[string]struct{}, len(item.RelatedReleases))
		for _, related := range item.RelatedReleases {
			if related.ProviderID <= 0 || (related.Relationship != "original" && related.Relationship != "remake" && related.Relationship != "remaster" && related.Relationship != "franchise") {
				return errors.New("backup contains an invalid game relationship")
			}
			key := fmt.Sprintf("%d:%s", related.ProviderID, related.Relationship)
			if _, duplicate := relationships[key]; duplicate {
				return errors.New("backup contains a duplicate game relationship")
			}
			relationships[key] = struct{}{}
			if err := textValue("related game title", related.Title, 1, 500); err != nil {
				return err
			}
			if err := optionalSecureURL("related game cover", related.CoverURL); err != nil {
				return err
			}
		}
	}

	for index := range document.Movies {
		item := &document.Movies[index]
		if err := validateEntity(item.ID, "movie", item.ProviderID, item.Title, entityIDs, movieProviders); err != nil {
			return err
		}
		if err := validateStates(item.Users, users, item.ID, progressEligible); err != nil {
			return err
		}
		if err := validateArtwork(item.ID, "tmdb", item.Artwork); err != nil {
			return err
		}
		if err := validatePins(item.Users, item.Artwork, false); err != nil {
			return err
		}
		artworkCount += len(item.Artwork)
		if err := validateCanonicalMetadata(item.ReleaseYear, item.CommunityRating, item.CommunityRatingCount, item.MetadataRefreshedAt); err != nil {
			return fmt.Errorf("backup contains invalid movie metadata: %w", err)
		}
		if item.RuntimeMinutes != nil && (*item.RuntimeMinutes < 1 || *item.RuntimeMinutes > 10_000) {
			return errors.New("backup contains an invalid movie runtime")
		}
		if err := validatePeople(append(append([]Person{}, item.Cast...), item.Crew...)); err != nil {
			return err
		}
		if (item.CollectionID == nil) != (item.CollectionName == nil) {
			return errors.New("backup movie collection identity is incomplete")
		}
		if item.CollectionID != nil && *item.CollectionID <= 0 {
			return errors.New("backup contains an invalid movie collection ID")
		}
		if item.Homepage != nil {
			if err := secureURL("movie homepage", *item.Homepage, false); err != nil {
				return err
			}
		}
		collectionIDs := make(map[int64]struct{}, len(item.Collection))
		for _, member := range item.Collection {
			if member.ProviderID <= 0 {
				return errors.New("backup contains an invalid movie collection member")
			}
			if _, duplicate := collectionIDs[member.ProviderID]; duplicate {
				return errors.New("backup contains a duplicate movie collection member")
			}
			collectionIDs[member.ProviderID] = struct{}{}
			if err := textValue("movie collection title", member.Title, 1, 500); err != nil {
				return err
			}
			if err := optionalSecureURL("movie collection poster", member.PosterURL); err != nil {
				return err
			}
		}
	}

	for index := range document.TVShows {
		item := &document.TVShows[index]
		if err := validateEntity(item.ID, "TV show", item.ProviderID, item.Title, entityIDs, tvProviders); err != nil {
			return err
		}
		if item.VerifiedTMDBID != nil {
			if *item.VerifiedTMDBID <= 0 || item.TMDBMappingVerifiedAt == nil {
				return errors.New("backup contains an invalid verified TV mapping")
			}
			if _, duplicate := verifiedTMDB[*item.VerifiedTMDBID]; duplicate {
				return errors.New("backup contains a duplicate verified TMDB TV ID")
			}
			verifiedTMDB[*item.VerifiedTMDBID] = struct{}{}
		} else if item.TMDBMappingVerifiedAt != nil {
			return errors.New("backup TV mapping timestamp has no verified TMDB ID")
		}
		if err := validateStates(item.Users, users, item.ID, progressEligible); err != nil {
			return err
		}
		if err := validateArtwork(item.ID, "tvdb", item.Artwork); err != nil {
			return err
		}
		if err := validatePins(item.Users, item.Artwork, false); err != nil {
			return err
		}
		artworkCount += len(item.Artwork)
		if err := validateCanonicalMetadata(item.ReleaseYear, item.CommunityRating, item.CommunityRatingCount, item.MetadataRefreshedAt); err != nil {
			return fmt.Errorf("backup contains invalid TV metadata: %w", err)
		}
		if err := validatePeople(append(append([]Person{}, item.Cast...), item.KeyPeople...)); err != nil {
			return err
		}
		seasonNumbers := make(map[int32]struct{})
		episodeNumbers := make(map[string]struct{})
		for _, season := range item.Seasons {
			if err := uuidv7("TV season ID", season.ID); err != nil {
				return err
			}
			if season.ProviderID <= 0 || season.Number < 0 || season.Special != (season.Number == 0) {
				return errors.New("backup contains invalid TV season metadata")
			}
			if _, duplicate := seasonIDs[season.ID]; duplicate {
				return errors.New("backup contains a duplicate TV season ID")
			}
			if _, duplicate := seasonProviderIDs[season.ProviderID]; duplicate {
				return errors.New("backup contains a duplicate TVDB season ID")
			}
			if _, duplicate := seasonNumbers[season.Number]; duplicate {
				return errors.New("backup contains a duplicate TV season number")
			}
			seasonIDs[season.ID], seasonProviderIDs[season.ProviderID], seasonNumbers[season.Number] = struct{}{}, struct{}{}, struct{}{}
			if err := optionalSecureURL("TV season poster", season.PosterURL); err != nil {
				return err
			}
			for _, episode := range season.Episodes {
				episodeCount++
				if err := uuidv7("TV episode ID", episode.ID); err != nil {
					return err
				}
				if episode.ProviderID <= 0 || episode.SeasonNumber != season.Number || episode.EpisodeNumber < 0 || episode.SortOrder < 0 || episode.Special != season.Special {
					return errors.New("backup contains invalid TV episode metadata")
				}
				if err := textValue("TV episode title", episode.Title, 1, 500); err != nil {
					return err
				}
				if _, duplicate := episodeIDs[episode.ID]; duplicate {
					return errors.New("backup contains a duplicate TV episode ID")
				}
				if _, duplicate := episodeProviderIDs[episode.ProviderID]; duplicate {
					return errors.New("backup contains a duplicate TVDB episode ID")
				}
				numberKey := fmt.Sprintf("%d:%d", episode.SeasonNumber, episode.EpisodeNumber)
				if _, duplicate := episodeNumbers[numberKey]; duplicate {
					return errors.New("backup contains a duplicate TV episode number")
				}
				episodeIDs[episode.ID] = item.ID
				episodeProviderIDs[episode.ProviderID], episodeNumbers[numberKey] = struct{}{}, struct{}{}
				if episode.RuntimeMinutes != nil && (*episode.RuntimeMinutes < 1 || *episode.RuntimeMinutes > 10_000) {
					return errors.New("backup contains an invalid TV episode runtime")
				}
				if err := optionalSecureURL("TV episode still", episode.StillURL); err != nil {
					return err
				}
			}
		}
	}
	if artworkCount > maxArtwork || episodeCount > maxEpisodes {
		return errors.New("backup contains too many media child records")
	}

	progressKeys := make(map[string]struct{}, len(document.EpisodeProgress))
	for _, progress := range document.EpisodeProgress {
		if _, ok := users[progress.UserID]; !ok {
			return errors.New("backup episode progress references an unknown user")
		}
		showID, ok := episodeIDs[progress.EpisodeID]
		if !ok || showID != progress.TVShowID {
			return errors.New("backup episode progress references an unknown episode")
		}
		if _, ok := progressEligible[progress.TVShowID][progress.UserID]; !ok {
			return errors.New("backup episode progress references an untracked TV show")
		}
		if progress.WatchedAt.IsZero() {
			return errors.New("backup episode progress is missing its watched time")
		}
		key := progress.UserID + "\x00" + progress.EpisodeID
		if _, duplicate := progressKeys[key]; duplicate {
			return errors.New("backup contains duplicate episode progress")
		}
		progressKeys[key] = struct{}{}
	}

	settingKeys := make(map[string]struct{}, len(document.Settings))
	for _, setting := range document.Settings {
		if setting.Key != "general.instance_name" {
			return errors.New("backup contains a setting that is not portable")
		}
		if _, duplicate := settingKeys[setting.Key]; duplicate {
			return errors.New("backup contains a duplicate setting")
		}
		settingKeys[setting.Key] = struct{}{}
		var value string
		if err := json.Unmarshal(setting.Value, &value); err != nil {
			return errors.New("backup contains an invalid portable setting")
		}
		if err := textValue("instance name", strings.TrimSpace(value), 1, 80); err != nil {
			return err
		}
	}
	return nil
}

func validateEntity[T ~int64](id, domain string, providerID T, title string, entityIDs map[string]string, providerIDs map[T]struct{}) error {
	if err := uuidv7(domain+" ID", id); err != nil {
		return err
	}
	if existing, duplicate := entityIDs[id]; duplicate {
		return fmt.Errorf("backup entity ID is shared by %s and %s", existing, domain)
	}
	entityIDs[id] = domain
	if providerID <= 0 {
		return fmt.Errorf("backup contains an invalid %s provider ID", domain)
	}
	if _, duplicate := providerIDs[providerID]; duplicate {
		return fmt.Errorf("backup contains a duplicate %s provider ID", domain)
	}
	providerIDs[providerID] = struct{}{}
	return textValue(domain+" title", title, 1, 500)
}

func validateStates(states []PersonalState, users map[string]struct{}, entityID string, eligible map[string]map[string]struct{}) error {
	seen := make(map[string]struct{}, len(states))
	for _, state := range states {
		if _, ok := users[state.UserID]; !ok {
			return errors.New("backup personal state references an unknown user")
		}
		if _, duplicate := seen[state.UserID]; duplicate {
			return errors.New("backup contains duplicate personal state")
		}
		seen[state.UserID] = struct{}{}
		status, err := media.ParseStatus(state.Status)
		if err != nil {
			return errors.New("backup contains an invalid personal status")
		}
		if _, err := media.ValidatePersonalState(media.PersonalState{Status: status, Rating: state.Rating, RatingReason: state.RatingReason}); err != nil {
			return fmt.Errorf("backup contains invalid personal state: %w", err)
		}
		if state.DateAdded.IsZero() {
			return errors.New("backup personal state is missing its date added")
		}
	}
	if eligible[entityID] == nil {
		eligible[entityID] = make(map[string]struct{}, len(states))
	}
	for userID := range seen {
		eligible[entityID][userID] = struct{}{}
	}
	return nil
}

func validateArtwork(entityID, provider string, artwork []Artwork) error {
	seen := make(map[string]struct{}, len(artwork))
	for _, item := range artwork {
		if item.Provider != provider || textValue("provider artwork ID", item.ProviderImageID, 1, 500) != nil {
			return fmt.Errorf("backup artwork for %s has invalid provider identity", entityID)
		}
		if _, duplicate := seen[item.ProviderImageID]; duplicate {
			return errors.New("backup contains duplicate provider artwork")
		}
		seen[item.ProviderImageID] = struct{}{}
		if err := media.ValidateArtworkKind(item.Kind); err != nil {
			return errors.New("backup contains an invalid artwork kind")
		}
		if err := secureURL("artwork URL", item.ImageURL, false); err != nil {
			return err
		}
		if err := secureURL("artwork thumbnail URL", item.ThumbnailURL, false); err != nil {
			return err
		}
		if item.Width != nil && *item.Width <= 0 || item.Height != nil && *item.Height <= 0 {
			return errors.New("backup artwork contains invalid dimensions")
		}
		if err := optionalText("artwork language", item.Language, 100); err != nil {
			return err
		}
	}
	return nil
}

func validateCanonicalMetadata(year *int32, rating *int16, ratingCount *int32, refreshedAt time.Time) error {
	if year != nil && (*year < 1800 || *year > 3000) {
		return errors.New("release year is outside the supported range")
	}
	if rating != nil && (*rating < 10 || *rating > 100) {
		return errors.New("community rating is outside the supported range")
	}
	if ratingCount != nil && *ratingCount < 0 {
		return errors.New("community rating count cannot be negative")
	}
	if refreshedAt.IsZero() {
		return errors.New("metadata refresh time is missing")
	}
	return nil
}

func validatePeople(people []Person) error {
	for _, person := range people {
		if err := textValue("person name", person.Name, 1, 500); err != nil {
			return err
		}
		if err := textValue("person role", person.Role, 1, 500); err != nil {
			return err
		}
		if person.ProfileURL != "" {
			if err := secureURL("person profile image", person.ProfileURL, false); err != nil {
				return err
			}
		}
		if person.ImageURL != "" {
			if err := secureURL("person image", person.ImageURL, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePins(states []PersonalState, artwork []Artwork, game bool) error {
	kinds := make(map[string]string, len(artwork))
	for _, item := range artwork {
		kinds[item.ProviderImageID] = item.Kind
	}
	for _, state := range states {
		checks := []struct {
			value *string
			kind  string
		}{
			{state.SelectedBackdropID, "backdrop"},
			{state.SelectedLogoID, "logo"},
		}
		if game {
			if state.SelectedPosterID != nil {
				return errors.New("backup game state contains a poster pin")
			}
			checks = append(checks, struct {
				value *string
				kind  string
			}{state.SelectedCoverID, "cover"})
		} else {
			if state.SelectedCoverID != nil {
				return errors.New("backup poster domain contains a cover pin")
			}
			checks = append(checks, struct {
				value *string
				kind  string
			}{state.SelectedPosterID, "poster"})
		}
		for _, check := range checks {
			if check.value != nil && kinds[*check.value] != check.kind {
				return errors.New("backup artwork pin does not reference matching provider artwork")
			}
		}
	}
	return nil
}

func uuidv7(label, value string) error {
	var identifier pgtype.UUID
	if err := identifier.Scan(value); err != nil || !identifier.Valid || identifier.Bytes[6]>>4 != 7 {
		return fmt.Errorf("backup contains an invalid UUIDv7 %s", label)
	}
	return nil
}

func secureURL(label, value string, allowLoopbackHTTP bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Fragment != "" {
		return fmt.Errorf("backup contains an invalid %s", label)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if allowLoopbackHTTP && parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		return nil
	}
	return fmt.Errorf("backup contains an insecure %s", label)
}

func optionalText(label string, value *string, maximum int) error {
	if value == nil {
		return nil
	}
	return textValue(label, *value, 0, maximum)
}

func optionalSecureURL(label string, value *string) error {
	if value == nil || *value == "" {
		return nil
	}
	return secureURL(label, *value, false)
}

func textValue(label, value string, minimum, maximum int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("backup %s is not valid UTF-8", label)
	}
	length := utf8.RuneCountInString(value)
	if length < minimum || length > maximum {
		return fmt.Errorf("backup %s has an invalid length", label)
	}
	return nil
}

func validSort(value string) bool {
	switch value {
	case "rating_desc", "rating_asc", "community_desc", "title_asc", "title_desc", "release_desc", "release_asc", "added_desc", "added_asc":
		return true
	default:
		return false
	}
}

func utcNow() time.Time { return time.Now().UTC() }
