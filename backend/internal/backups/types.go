package backups

import (
	"encoding/json"
	"time"
)

const (
	Format        = "gradeium-backup"
	FormatVersion = 1
)

type Kind string

const (
	KindManual     Kind = "manual"
	KindAutomatic  Kind = "automatic"
	KindPreRestore Kind = "pre_restore"
)

type Document struct {
	Format             string     `json:"format"`
	Version            int        `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	ApplicationVersion string     `json:"applicationVersion"`
	Users              []User     `json:"users"`
	Games              []Game     `json:"games"`
	Movies             []Movie    `json:"movies"`
	TVShows            []TVShow   `json:"tvShows"`
	EpisodeProgress    []Progress `json:"episodeProgress"`
	Settings           []Setting  `json:"settings"`
}

type User struct {
	ID          string      `json:"id"`
	OIDCIssuer  *string     `json:"oidcIssuer,omitempty"`
	OIDCSubject *string     `json:"oidcSubject,omitempty"`
	DisplayName *string     `json:"displayName,omitempty"`
	Email       *string     `json:"email,omitempty"`
	Preferences Preferences `json:"preferences"`
}

type Preferences struct {
	DefaultLibrarySort string `json:"defaultLibrarySort"`
	PreferredView      string `json:"preferredView"`
	Theme              string `json:"theme"`
}

type Setting struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type PersonalState struct {
	UserID             string    `json:"userId"`
	Status             string    `json:"status"`
	Rating             *int16    `json:"rating,omitempty"`
	RatingReason       *string   `json:"ratingReason,omitempty"`
	DateAdded          time.Time `json:"dateAdded"`
	SelectedPosterID   *string   `json:"selectedPosterId,omitempty"`
	SelectedCoverID    *string   `json:"selectedCoverId,omitempty"`
	SelectedBackdropID *string   `json:"selectedBackdropId,omitempty"`
	SelectedLogoID     *string   `json:"selectedLogoId,omitempty"`
}

type Artwork struct {
	Provider        string  `json:"provider"`
	ProviderImageID string  `json:"providerImageId"`
	Kind            string  `json:"kind"`
	Language        *string `json:"language,omitempty"`
	ImageURL        string  `json:"imageUrl"`
	ThumbnailURL    string  `json:"thumbnailUrl"`
	Width           *int32  `json:"width,omitempty"`
	Height          *int32  `json:"height,omitempty"`
	Preferred       bool    `json:"preferred"`
	Available       bool    `json:"available"`
	SortOrder       int32   `json:"sortOrder"`
}

type ExternalLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Person struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	ProfileURL string `json:"profileUrl,omitempty"`
	ImageURL   string `json:"imageUrl,omitempty"`
}

type Game struct {
	ID                   string             `json:"id"`
	ProviderID           int64              `json:"providerId"`
	Title                string             `json:"title"`
	OriginalTitle        *string            `json:"originalTitle,omitempty"`
	Summary              *string            `json:"summary,omitempty"`
	ReleaseDate          *time.Time         `json:"releaseDate,omitempty"`
	ReleaseYear          *int32             `json:"releaseYear,omitempty"`
	GameType             string             `json:"gameType"`
	Developer            *string            `json:"developer,omitempty"`
	Publisher            *string            `json:"publisher,omitempty"`
	Genres               []string           `json:"genres"`
	GameModes            []string           `json:"gameModes"`
	Platforms            []string           `json:"platforms"`
	Franchise            *string            `json:"franchise,omitempty"`
	CommunityRating      *int16             `json:"communityRating,omitempty"`
	CommunityRatingCount *int32             `json:"communityRatingCount,omitempty"`
	Screenshots          []string           `json:"screenshots"`
	ExternalLinks        []ExternalLink     `json:"externalLinks"`
	MetadataRefreshedAt  time.Time          `json:"metadataRefreshedAt"`
	Artwork              []Artwork          `json:"artwork"`
	AdditionalContent    []GameContent      `json:"additionalContent"`
	RelatedReleases      []GameRelationship `json:"relatedReleases"`
	Users                []PersonalState    `json:"users"`
}

type GameContent struct {
	ProviderID int64   `json:"providerId"`
	Title      string  `json:"title"`
	Type       string  `json:"type"`
	Year       *int32  `json:"year,omitempty"`
	CoverURL   *string `json:"coverUrl,omitempty"`
}

type GameRelationship struct {
	ProviderID   int64   `json:"providerId"`
	Title        string  `json:"title"`
	Relationship string  `json:"relationship"`
	Year         *int32  `json:"year,omitempty"`
	CoverURL     *string `json:"coverUrl,omitempty"`
}

type Movie struct {
	ID                   string             `json:"id"`
	ProviderID           int64              `json:"providerId"`
	Title                string             `json:"title"`
	OriginalTitle        *string            `json:"originalTitle,omitempty"`
	Overview             *string            `json:"overview,omitempty"`
	ReleaseDate          *time.Time         `json:"releaseDate,omitempty"`
	ReleaseYear          *int32             `json:"releaseYear,omitempty"`
	RuntimeMinutes       *int32             `json:"runtimeMinutes,omitempty"`
	Director             *string            `json:"director,omitempty"`
	Genres               []string           `json:"genres"`
	ProductionCompanies  []string           `json:"productionCompanies"`
	Cast                 []Person           `json:"cast"`
	Crew                 []Person           `json:"crew"`
	TrailerKey           *string            `json:"trailerKey,omitempty"`
	IMDbID               *string            `json:"imdbId,omitempty"`
	Homepage             *string            `json:"homepage,omitempty"`
	CollectionID         *int64             `json:"collectionId,omitempty"`
	CollectionName       *string            `json:"collectionName,omitempty"`
	CommunityRating      *int16             `json:"communityRating,omitempty"`
	CommunityRatingCount *int32             `json:"communityRatingCount,omitempty"`
	MetadataRefreshedAt  time.Time          `json:"metadataRefreshedAt"`
	Artwork              []Artwork          `json:"artwork"`
	Collection           []CollectionMember `json:"collection"`
	Users                []PersonalState    `json:"users"`
}

type CollectionMember struct {
	ProviderID  int64      `json:"providerId"`
	Title       string     `json:"title"`
	ReleaseDate *time.Time `json:"releaseDate,omitempty"`
	PosterURL   *string    `json:"posterUrl,omitempty"`
}

type TVShow struct {
	ID                         string          `json:"id"`
	ProviderID                 int64           `json:"providerId"`
	VerifiedTMDBID             *int64          `json:"verifiedTmdbId,omitempty"`
	Title                      string          `json:"title"`
	OriginalTitle              *string         `json:"originalTitle,omitempty"`
	Overview                   *string         `json:"overview,omitempty"`
	FirstAired                 *time.Time      `json:"firstAired,omitempty"`
	ReleaseYear                *int32          `json:"releaseYear,omitempty"`
	ProviderStatus             *string         `json:"providerStatus,omitempty"`
	Network                    *string         `json:"network,omitempty"`
	Genres                     []string        `json:"genres"`
	Cast                       []Person        `json:"cast"`
	KeyPeople                  []Person        `json:"keyPeople"`
	CommunityRating            *int16          `json:"communityRating,omitempty"`
	CommunityRatingCount       *int32          `json:"communityRatingCount,omitempty"`
	TMDBMappingVerifiedAt      *time.Time      `json:"tmdbMappingVerifiedAt,omitempty"`
	MetadataRefreshedAt        time.Time       `json:"metadataRefreshedAt"`
	CommunityRatingRefreshedAt *time.Time      `json:"communityRatingRefreshedAt,omitempty"`
	Artwork                    []Artwork       `json:"artwork"`
	Seasons                    []Season        `json:"seasons"`
	Users                      []PersonalState `json:"users"`
}

type Season struct {
	ID         string     `json:"id"`
	ProviderID int64      `json:"providerId"`
	Number     int32      `json:"number"`
	Name       *string    `json:"name,omitempty"`
	Special    bool       `json:"special"`
	AirDate    *time.Time `json:"airDate,omitempty"`
	PosterURL  *string    `json:"posterUrl,omitempty"`
	Available  bool       `json:"available"`
	Episodes   []Episode  `json:"episodes"`
}

type Episode struct {
	ID             string     `json:"id"`
	ProviderID     int64      `json:"providerId"`
	SeasonNumber   int32      `json:"seasonNumber"`
	EpisodeNumber  int32      `json:"episodeNumber"`
	SortOrder      int32      `json:"sortOrder"`
	Title          string     `json:"title"`
	Overview       *string    `json:"overview,omitempty"`
	AirDate        *time.Time `json:"airDate,omitempty"`
	RuntimeMinutes *int32     `json:"runtimeMinutes,omitempty"`
	StillURL       *string    `json:"stillUrl,omitempty"`
	Special        bool       `json:"special"`
	Available      bool       `json:"available"`
}

type Progress struct {
	UserID    string    `json:"userId"`
	TVShowID  string    `json:"tvShowId"`
	EpisodeID string    `json:"episodeId"`
	WatchedAt time.Time `json:"watchedAt"`
}

type Metadata struct {
	ID                 string    `json:"id"`
	Filename           string    `json:"filename"`
	Kind               Kind      `json:"kind"`
	CreatedAt          time.Time `json:"createdAt"`
	SizeBytes          int64     `json:"sizeBytes"`
	SHA256             string    `json:"sha256"`
	FormatVersion      int       `json:"formatVersion"`
	ApplicationVersion string    `json:"applicationVersion"`
	Valid              bool      `json:"valid"`
}

type Settings struct {
	Enabled                   bool       `json:"enabled"`
	IntervalDays              int32      `json:"intervalDays"`
	RetentionCount            int32      `json:"retentionCount"`
	LastAttemptAt             *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessfulAutomaticAt *time.Time `json:"lastSuccessfulAutomaticAt,omitempty"`
	NextDueAt                 *time.Time `json:"nextDueAt,omitempty"`
	LastError                 *string    `json:"lastError,omitempty"`
}
