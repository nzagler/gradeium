package igdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
	"github.com/nzagler/gradeium/backend/internal/media"
)

const (
	defaultTokenURL = "https://id.twitch.tv/oauth2/token"
	defaultAPIURL   = "https://api.igdb.com/v4"
	searchLimit     = 20
)

var ErrNotTrackable = errors.New("this IGDB record is an edition or non-independent game type and cannot be tracked independently")

type Client struct {
	http         *provider.Client
	clientID     string
	clientSecret string
	tokenURL     string
	apiURL       string
	mu           sync.Mutex
	token        string
	tokenExpiry  time.Time
}

func NewClient(clientID, clientSecret string) *Client {
	return NewClientWithEndpoints(provider.NewClient(), clientID, clientSecret, defaultTokenURL, defaultAPIURL)
}

func NewClientWithEndpoints(httpClient *provider.Client, clientID, clientSecret, tokenURL, apiURL string) *Client {
	return &Client{
		http: httpClient, clientID: strings.TrimSpace(clientID), clientSecret: clientSecret,
		tokenURL: strings.TrimRight(tokenURL, "/"), apiURL: strings.TrimRight(apiURL, "/"),
	}
}

type SearchResult struct {
	ProviderID int64  `json:"providerId"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	Developer  string `json:"developer,omitempty"`
	GameType   string `json:"gameType,omitempty"`
	ArtworkURL string `json:"artworkUrl,omitempty"`
}

type SearchPage struct {
	Results []SearchResult `json:"results"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
}

type AdditionalContent struct {
	ProviderID int64  `json:"providerId"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Year       *int   `json:"year,omitempty"`
	CoverURL   string `json:"coverUrl,omitempty"`
}

type RelatedRelease struct {
	ProviderID   int64  `json:"providerId"`
	Title        string `json:"title"`
	Relationship string `json:"relationship"`
	Year         *int   `json:"year,omitempty"`
	CoverURL     string `json:"coverUrl,omitempty"`
}

type Game struct {
	ProviderID           int64               `json:"providerId"`
	Title                string              `json:"title"`
	OriginalTitle        string              `json:"originalTitle,omitempty"`
	Summary              string              `json:"summary,omitempty"`
	ReleaseDate          *time.Time          `json:"releaseDate,omitempty"`
	Year                 *int                `json:"year,omitempty"`
	GameType             string              `json:"gameType"`
	Developer            string              `json:"developer,omitempty"`
	Publisher            string              `json:"publisher,omitempty"`
	Genres               []string            `json:"genres"`
	GameModes            []string            `json:"gameModes"`
	Platforms            []string            `json:"platforms"`
	Franchise            string              `json:"franchise,omitempty"`
	CommunityRating      *int16              `json:"communityRating,omitempty"`
	CommunityRatingCount *int32              `json:"communityRatingCount,omitempty"`
	Artworks             []media.Artwork     `json:"artworks"`
	Screenshots          []string            `json:"screenshots"`
	AdditionalContent    []AdditionalContent `json:"additionalContent"`
	RelatedReleases      []RelatedRelease    `json:"relatedReleases"`
	ExternalLinks        []ExternalLink      `json:"externalLinks"`
}

type ExternalLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type gameDTO struct {
	ID               int64            `json:"id"`
	Name             string           `json:"name"`
	Summary          string           `json:"summary"`
	FirstReleaseDate int64            `json:"first_release_date"`
	Rating           float64          `json:"rating"`
	RatingCount      int32            `json:"rating_count"`
	GameType         namedTypeDTO     `json:"game_type"`
	ParentGame       json.RawMessage  `json:"parent_game"`
	VersionParent    json.RawMessage  `json:"version_parent"`
	Cover            imageDTO         `json:"cover"`
	Artworks         []imageDTO       `json:"artworks"`
	Screenshots      []imageDTO       `json:"screenshots"`
	Genres           []namedDTO       `json:"genres"`
	GameModes        []namedDTO       `json:"game_modes"`
	Platforms        []namedDTO       `json:"platforms"`
	Companies        []companyLinkDTO `json:"involved_companies"`
	Franchises       []namedDTO       `json:"franchises"`
	Websites         []websiteDTO     `json:"websites"`
	DLCs             []gameRefDTO     `json:"dlcs"`
	Expansions       []gameRefDTO     `json:"expansions"`
	Bundles          []gameRefDTO     `json:"bundles"`
	Remakes          []gameRefDTO     `json:"remakes"`
	Remasters        []gameRefDTO     `json:"remasters"`
	ExpandedGames    []gameRefDTO     `json:"expanded_games"`
}

type namedDTO struct {
	Name string `json:"name"`
}
type namedTypeDTO struct {
	Type string `json:"type"`
}
type imageDTO struct {
	ID      int64  `json:"id"`
	ImageID string `json:"image_id"`
	Width   int32  `json:"width"`
	Height  int32  `json:"height"`
}
type companyLinkDTO struct {
	Developer bool     `json:"developer"`
	Publisher bool     `json:"publisher"`
	Company   namedDTO `json:"company"`
}
type websiteDTO struct {
	URL  string       `json:"url"`
	Type namedTypeDTO `json:"type"`
}
type gameRefDTO struct {
	ID               int64        `json:"id"`
	Name             string       `json:"name"`
	FirstReleaseDate int64        `json:"first_release_date"`
	GameType         namedTypeDTO `json:"game_type"`
	Cover            imageDTO     `json:"cover"`
}

func (client *Client) Test(ctx context.Context) error {
	if _, err := client.accessToken(ctx); err != nil {
		return err
	}
	var games []gameDTO
	return client.query(ctx, "games", "fields id; limit 1;", &games)
}

func (client *Client) Search(ctx context.Context, query string, page int) (SearchPage, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return SearchPage{}, errors.New("search query must contain at least 2 characters")
	}
	if page < 1 || page > 100 {
		return SearchPage{}, errors.New("search page is out of range")
	}
	statement := fmt.Sprintf(
		"search %s; fields id,name,first_release_date,game_type.type,version_parent,cover.image_id,involved_companies.developer,involved_companies.company.name; where version_parent = null; limit %d; offset %d;",
		quoteQuery(query), searchLimit, (page-1)*searchLimit,
	)
	var rows []gameDTO
	if err := client.query(ctx, "games", statement, &rows); err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		if !independentlyTrackable(row) || row.ID <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		developer, _ := companies(row.Companies)
		result := SearchResult{ProviderID: row.ID, Title: row.Name, Developer: developer, GameType: displayType(row.GameType.Type)}
		result.Year = yearFromUnix(row.FirstReleaseDate)
		if row.Cover.ImageID != "" {
			result.ArtworkURL = igdbImage(row.Cover.ImageID, "t_cover_big")
		}
		results = append(results, result)
	}
	return SearchPage{Results: results, Page: page, HasMore: len(rows) == searchLimit}, nil
}

func (client *Client) Game(ctx context.Context, providerID int64) (Game, error) {
	if providerID <= 0 {
		return Game{}, errors.New("invalid IGDB game ID")
	}
	fields := "id,name,summary,first_release_date,rating,rating_count,game_type.type,parent_game,version_parent," +
		"cover.id,cover.image_id,cover.width,cover.height,artworks.id,artworks.image_id,artworks.width,artworks.height," +
		"screenshots.image_id,genres.name,game_modes.name,platforms.name," +
		"involved_companies.developer,involved_companies.publisher,involved_companies.company.name," +
		"franchises.name,websites.url,websites.type.type," +
		"dlcs.id,dlcs.name,dlcs.first_release_date,dlcs.game_type.type,dlcs.cover.image_id," +
		"expansions.id,expansions.name,expansions.first_release_date,expansions.game_type.type,expansions.cover.image_id," +
		"bundles.id,bundles.name,bundles.first_release_date,bundles.game_type.type,bundles.cover.image_id," +
		"remakes.id,remakes.name,remakes.first_release_date,remakes.cover.image_id," +
		"remasters.id,remasters.name,remasters.first_release_date,remasters.cover.image_id," +
		"expanded_games.id,expanded_games.name,expanded_games.first_release_date,expanded_games.cover.image_id"
	var rows []gameDTO
	if err := client.query(ctx, "games", fmt.Sprintf("fields %s; where id = %d; limit 1;", fields, providerID), &rows); err != nil {
		return Game{}, err
	}
	if len(rows) != 1 {
		return Game{}, errors.New("IGDB game was not found")
	}
	row := rows[0]
	if !independentlyTrackable(row) {
		return Game{}, ErrNotTrackable
	}
	developer, publisher := companies(row.Companies)
	game := Game{
		ProviderID: row.ID, Title: strings.TrimSpace(row.Name), Summary: strings.TrimSpace(row.Summary),
		GameType: displayType(row.GameType.Type), Developer: developer, Publisher: publisher,
		Genres: names(row.Genres), GameModes: names(row.GameModes), Platforms: names(row.Platforms),
		CommunityRating: media.CommunityRating(row.Rating, 100), Artworks: make([]media.Artwork, 0),
		Screenshots: make([]string, 0), AdditionalContent: make([]AdditionalContent, 0),
		RelatedReleases: make([]RelatedRelease, 0), ExternalLinks: safeLinks(row.Websites),
	}
	game.ReleaseDate, game.Year = dateAndYear(row.FirstReleaseDate)
	if row.RatingCount > 0 {
		count := row.RatingCount
		game.CommunityRatingCount = &count
	}
	if len(row.Franchises) > 0 {
		game.Franchise = row.Franchises[0].Name
	}
	if row.Cover.ImageID != "" {
		game.Artworks = append(game.Artworks, artwork(row.Cover, "cover", true))
	}
	for index, value := range row.Artworks {
		item := artwork(value, "backdrop", index == 0)
		game.Artworks = append(game.Artworks, item)
	}
	for _, screenshot := range row.Screenshots {
		if screenshot.ImageID != "" && len(game.Screenshots) < 5 {
			game.Screenshots = append(game.Screenshots, igdbImage(screenshot.ImageID, "t_1080p"))
		}
	}
	for _, group := range [][]gameRefDTO{row.DLCs, row.Expansions, row.Bundles} {
		for _, value := range group {
			game.AdditionalContent = append(game.AdditionalContent, additional(value))
		}
	}
	for _, value := range row.Remakes {
		game.RelatedReleases = append(game.RelatedReleases, related(value, "remake"))
	}
	for _, value := range row.Remasters {
		game.RelatedReleases = append(game.RelatedReleases, related(value, "remaster"))
	}
	for _, value := range row.ExpandedGames {
		game.RelatedReleases = append(game.RelatedReleases, related(value, "franchise"))
	}
	return game, nil
}

func (client *Client) query(ctx context.Context, endpoint, statement string, destination any) error {
	token, err := client.accessToken(ctx)
	if err != nil {
		return err
	}
	headers := make(http.Header)
	headers.Set("Client-ID", client.clientID)
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("Content-Type", "text/plain")
	if err := client.http.JSON(ctx, http.MethodPost, client.apiURL+"/"+endpoint, headers, []byte(statement), destination, true); err != nil {
		return fmt.Errorf("IGDB request failed: %w", err)
	}
	return nil
}

func (client *Client) accessToken(ctx context.Context) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.token != "" && time.Now().Add(time.Minute).Before(client.tokenExpiry) {
		return client.token, nil
	}
	if client.clientID == "" || client.clientSecret == "" {
		return "", errors.New("IGDB credentials are not configured")
	}
	values := url.Values{"client_id": {client.clientID}, "client_secret": {client.clientSecret}, "grant_type": {"client_credentials"}}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := client.http.JSON(ctx, http.MethodPost, client.tokenURL, headers, []byte(values.Encode()), &result, false); err != nil {
		return "", fmt.Errorf("IGDB authentication failed: %w", err)
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 {
		return "", errors.New("IGDB authentication returned invalid data")
	}
	client.token = result.AccessToken
	client.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return client.token, nil
}

func TrackableGameType(value string) bool {
	value = normalizeType(value)
	switch value {
	case "main game", "standalone expansion", "remake", "remaster", "fork", "episode":
		return true
	default:
		return false
	}
}

func independentlyTrackable(game gameDTO) bool {
	return TrackableGameType(game.GameType.Type) && !hasReference(game.VersionParent)
}

func normalizeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), " ")
}

func displayType(value string) string {
	value = normalizeType(value)
	if value == "main game" || value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for index := range parts {
		parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
	}
	return strings.Join(parts, " ")
}

func quoteQuery(value string) string {
	value = strings.ReplaceAll(value, "\\", " ")
	value = strings.ReplaceAll(value, "\"", " ")
	value = strings.Join(strings.Fields(value), " ")
	return strconv.Quote(value)
}

func hasReference(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "0" && trimmed != "{}"
}

func names(values []namedDTO) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func companies(values []companyLinkDTO) (string, string) {
	var developer, publisher string
	for _, value := range values {
		if developer == "" && value.Developer {
			developer = strings.TrimSpace(value.Company.Name)
		}
		if publisher == "" && value.Publisher {
			publisher = strings.TrimSpace(value.Company.Name)
		}
	}
	return developer, publisher
}

func yearFromUnix(timestamp int64) *int { _, year := dateAndYear(timestamp); return year }
func dateAndYear(timestamp int64) (*time.Time, *int) {
	if timestamp <= 0 {
		return nil, nil
	}
	date := time.Unix(timestamp, 0).UTC()
	year := date.Year()
	return &date, &year
}

func igdbImage(imageID, size string) string {
	return "https://images.igdb.com/igdb/image/upload/" + size + "/" + url.PathEscape(imageID) + ".jpg"
}

func artwork(value imageDTO, kind string, preferred bool) media.Artwork {
	size := "t_1080p"
	thumb := "t_screenshot_med"
	if kind == "cover" {
		size = "t_cover_big"
		thumb = "t_cover_small"
	}
	return media.Artwork{ProviderImageID: value.ImageID, Kind: kind, ImageURL: igdbImage(value.ImageID, size), ThumbnailURL: igdbImage(value.ImageID, thumb), Width: value.Width, Height: value.Height, Preferred: preferred, Available: true}
}

func additional(value gameRefDTO) AdditionalContent {
	result := AdditionalContent{ProviderID: value.ID, Title: value.Name, Type: displayType(value.GameType.Type)}
	result.Year = yearFromUnix(value.FirstReleaseDate)
	if value.Cover.ImageID != "" {
		result.CoverURL = igdbImage(value.Cover.ImageID, "t_cover_small")
	}
	return result
}

func related(value gameRefDTO, relationship string) RelatedRelease {
	result := RelatedRelease{ProviderID: value.ID, Title: value.Name, Relationship: relationship, Year: yearFromUnix(value.FirstReleaseDate)}
	if value.Cover.ImageID != "" {
		result.CoverURL = igdbImage(value.Cover.ImageID, "t_cover_small")
	}
	return result
}

func safeLinks(values []websiteDTO) []ExternalLink {
	result := make([]ExternalLink, 0, len(values))
	for _, value := range values {
		parsed, err := url.Parse(value.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		label := strings.TrimSpace(value.Type.Type)
		lower := strings.ToLower(label + " " + parsed.Host)
		switch {
		case strings.Contains(lower, "official"):
			label = "Official site"
		case strings.Contains(lower, "steam"):
			label = "Steam"
		case strings.Contains(lower, "gog"):
			label = "GOG"
		case strings.Contains(lower, "epic"):
			label = "Epic"
		case strings.Contains(lower, "wikipedia"):
			label = "Wikipedia"
		default:
			continue
		}
		result = append(result, ExternalLink{Label: label, URL: parsed.String()})
	}
	return result
}
