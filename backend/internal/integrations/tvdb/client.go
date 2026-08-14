package tvdb

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

const defaultAPIURL = "https://api4.thetvdb.com/v4"

type Client struct {
	http   *provider.Client
	apiKey string
	pin    string
	apiURL string
	mu     sync.Mutex
	token  string
}

func NewClient(apiKey, pin string) *Client {
	return NewClientWithEndpoint(provider.NewClient(), apiKey, pin, defaultAPIURL)
}

func NewClientWithEndpoint(httpClient *provider.Client, apiKey, pin, apiURL string) *Client {
	return &Client{http: httpClient, apiKey: strings.TrimSpace(apiKey), pin: strings.TrimSpace(pin), apiURL: strings.TrimRight(apiURL, "/")}
}

type SearchResult struct {
	ProviderID int64  `json:"providerId"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	Network    string `json:"network,omitempty"`
	ArtworkURL string `json:"artworkUrl,omitempty"`
}
type SearchPage struct {
	Results []SearchResult `json:"results"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
}
type Person struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	ImageURL string `json:"imageUrl,omitempty"`
}
type Episode struct {
	ProviderID     int64      `json:"providerId"`
	SeasonNumber   int32      `json:"seasonNumber"`
	EpisodeNumber  int32      `json:"episodeNumber"`
	SortOrder      int32      `json:"sortOrder"`
	Title          string     `json:"title"`
	Overview       string     `json:"overview,omitempty"`
	AirDate        *time.Time `json:"airDate,omitempty"`
	RuntimeMinutes *int32     `json:"runtimeMinutes,omitempty"`
	StillURL       string     `json:"stillUrl,omitempty"`
	Special        bool       `json:"special"`
}
type Season struct {
	ProviderID int64      `json:"providerId"`
	Number     int32      `json:"number"`
	Name       string     `json:"name,omitempty"`
	Special    bool       `json:"special"`
	AirDate    *time.Time `json:"airDate,omitempty"`
	PosterURL  string     `json:"posterUrl,omitempty"`
	Episodes   []Episode  `json:"episodes"`
}
type Show struct {
	ProviderID     int64           `json:"providerId"`
	Title          string          `json:"title"`
	OriginalTitle  string          `json:"originalTitle,omitempty"`
	Overview       string          `json:"overview,omitempty"`
	FirstAired     *time.Time      `json:"firstAired,omitempty"`
	Year           *int            `json:"year,omitempty"`
	ProviderStatus string          `json:"providerStatus,omitempty"`
	Network        string          `json:"network,omitempty"`
	Genres         []string        `json:"genres"`
	Cast           []Person        `json:"cast"`
	KeyPeople      []Person        `json:"keyPeople"`
	Artworks       []media.Artwork `json:"artworks"`
	Seasons        []Season        `json:"seasons"`
}

type envelope[T any] struct {
	Status string   `json:"status"`
	Data   T        `json:"data"`
	Links  linksDTO `json:"links"`
}
type linksDTO struct {
	Next json.RawMessage `json:"next"`
}
type searchDTO struct {
	ObjectID     string `json:"objectID"`
	TVDBID       string `json:"tvdb_id"`
	Name         string `json:"name"`
	FirstAirTime string `json:"first_air_time"`
	Year         string `json:"year"`
	ImageURL     string `json:"image_url"`
	Network      string `json:"network"`
}
type namedDTO struct {
	Name string `json:"name"`
}
type statusDTO struct {
	Name string `json:"name"`
}
type companyTypeDTO struct {
	CompanyTypeName string `json:"companyTypeName"`
	CompanyTypeID   int64  `json:"companyTypeId"`
}
type companyDTO struct {
	Name        string         `json:"name"`
	CompanyType companyTypeDTO `json:"companyType"`
}
type artworkDTO struct {
	ID        int64   `json:"id"`
	Image     string  `json:"image"`
	Thumbnail string  `json:"thumbnail"`
	Language  string  `json:"language"`
	Type      int32   `json:"type"`
	Width     int32   `json:"width"`
	Height    int32   `json:"height"`
	Score     float64 `json:"score"`
}
type characterDTO struct {
	Name       string `json:"name"`
	PersonName string `json:"personName"`
	Image      string `json:"image"`
	PeopleType string `json:"peopleType"`
	Sort       int32  `json:"sort"`
}
type seasonTypeDTO struct {
	Type string `json:"type"`
}
type seasonDTO struct {
	ID         int64         `json:"id"`
	Number     int32         `json:"number"`
	Name       string        `json:"name"`
	Image      string        `json:"image"`
	FirstAired string        `json:"firstAired"`
	Year       string        `json:"year"`
	Type       seasonTypeDTO `json:"type"`
}
type showDTO struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	Overview        string         `json:"overview"`
	FirstAired      string         `json:"firstAired"`
	Year            string         `json:"year"`
	Status          statusDTO      `json:"status"`
	OriginalNetwork *namedDTO      `json:"originalNetwork"`
	LatestNetwork   *namedDTO      `json:"latestNetwork"`
	Genres          []namedDTO     `json:"genres"`
	Companies       []companyDTO   `json:"companies"`
	Artworks        []artworkDTO   `json:"artworks"`
	Characters      []characterDTO `json:"characters"`
	Seasons         []seasonDTO    `json:"seasons"`
}
type episodeDTO struct {
	ID             int64  `json:"id"`
	SeasonNumber   int32  `json:"seasonNumber"`
	Number         int32  `json:"number"`
	Name           string `json:"name"`
	Overview       string `json:"overview"`
	Aired          string `json:"aired"`
	Runtime        int32  `json:"runtime"`
	Image          string `json:"image"`
	AbsoluteNumber int32  `json:"absoluteNumber"`
}
type episodesDTO struct {
	Series   showDTO      `json:"series"`
	Episodes []episodeDTO `json:"episodes"`
}

func (client *Client) Test(ctx context.Context) error {
	if _, err := client.accessToken(ctx); err != nil {
		return err
	}
	var languages envelope[[]json.RawMessage]
	return client.get(ctx, "/languages", nil, &languages)
}

func (client *Client) Search(ctx context.Context, query string, page int) (SearchPage, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return SearchPage{}, errors.New("search query must contain at least 2 characters")
	}
	if page < 1 || page > 100 {
		return SearchPage{}, errors.New("search page is out of range")
	}
	parameters := url.Values{"query": {query}, "type": {"series"}, "language": {"eng"}, "limit": {"20"}, "page": {strconv.Itoa(page - 1)}}
	var response envelope[[]searchDTO]
	if err := client.get(ctx, "/search", parameters, &response); err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, len(response.Data))
	for _, row := range response.Data {
		id, _ := strconv.ParseInt(firstNonEmpty(row.TVDBID, row.ObjectID), 10, 64)
		if id <= 0 || strings.TrimSpace(row.Name) == "" {
			continue
		}
		item := SearchResult{ProviderID: id, Title: row.Name, Network: row.Network, ArtworkURL: safeImage(row.ImageURL)}
		item.Year = parseYear(firstNonEmpty(row.Year, row.FirstAirTime))
		results = append(results, item)
	}
	return SearchPage{Results: results, Page: page, HasMore: hasNext(response.Links.Next)}, nil
}

func (client *Client) Show(ctx context.Context, providerID int64) (Show, error) {
	if providerID <= 0 {
		return Show{}, errors.New("invalid TVDB series ID")
	}
	var response envelope[showDTO]
	if err := client.get(ctx, "/series/"+strconv.FormatInt(providerID, 10)+"/extended", url.Values{"meta": {"translations"}, "short": {"true"}}, &response); err != nil {
		return Show{}, err
	}
	if response.Data.ID != providerID || strings.TrimSpace(response.Data.Name) == "" {
		return Show{}, errors.New("TVDB returned invalid series data")
	}
	row := response.Data
	show := Show{ProviderID: row.ID, Title: row.Name, Overview: strings.TrimSpace(row.Overview), FirstAired: parseDate(row.FirstAired), Year: parseYear(firstNonEmpty(row.Year, row.FirstAired)), ProviderStatus: row.Status.Name, Network: network(row), Genres: names(row.Genres), Cast: cast(row.Characters), KeyPeople: []Person{}, Artworks: artworks(row.Artworks), Seasons: []Season{}}
	episodes, err := client.episodes(ctx, providerID)
	if err != nil {
		return Show{}, err
	}
	seasonByNumber := map[int32]int{}
	for _, rowSeason := range row.Seasons {
		if rowSeason.ID <= 0 || !isDefaultSeason(rowSeason.Type.Type) {
			continue
		}
		season := Season{ProviderID: rowSeason.ID, Number: rowSeason.Number, Name: rowSeason.Name, Special: rowSeason.Number == 0, AirDate: seasonAirDate(rowSeason), PosterURL: safeImage(rowSeason.Image), Episodes: []Episode{}}
		show.Seasons = append(show.Seasons, season)
		seasonByNumber[season.Number] = len(show.Seasons) - 1
	}
	for index, value := range episodes {
		seasonIndex, found := seasonByNumber[value.SeasonNumber]
		if !found {
			show.Seasons = append(show.Seasons, Season{ProviderID: syntheticSeasonID(providerID, value.SeasonNumber), Number: value.SeasonNumber, Special: value.SeasonNumber == 0, Episodes: []Episode{}})
			seasonIndex = len(show.Seasons) - 1
			seasonByNumber[value.SeasonNumber] = seasonIndex
		}
		episode := Episode{ProviderID: value.ID, SeasonNumber: value.SeasonNumber, EpisodeNumber: value.Number, SortOrder: int32(index), Title: firstNonEmpty(value.Name, fmt.Sprintf("Episode %d", value.Number)), Overview: strings.TrimSpace(value.Overview), AirDate: parseDate(value.Aired), StillURL: safeImage(value.Image), Special: value.SeasonNumber == 0}
		if value.Runtime > 0 {
			runtime := value.Runtime
			episode.RuntimeMinutes = &runtime
		}
		show.Seasons[seasonIndex].Episodes = append(show.Seasons[seasonIndex].Episodes, episode)
	}
	sort.Slice(show.Seasons, func(i, j int) bool { return show.Seasons[i].Number < show.Seasons[j].Number })
	for index := range show.Seasons {
		sort.Slice(show.Seasons[index].Episodes, func(i, j int) bool {
			return show.Seasons[index].Episodes[i].EpisodeNumber < show.Seasons[index].Episodes[j].EpisodeNumber
		})
	}
	return show, nil
}

func seasonAirDate(value seasonDTO) *time.Time {
	if date := parseDate(value.FirstAired); date != nil {
		return date
	}
	year := parseYear(value.Year)
	if year == nil {
		return nil
	}
	date := time.Date(*year, time.January, 1, 0, 0, 0, 0, time.UTC)
	return &date
}

func (client *Client) episodes(ctx context.Context, providerID int64) ([]episodeDTO, error) {
	result := make([]episodeDTO, 0)
	for page := 0; page < 50; page++ {
		parameters := url.Values{"page": {strconv.Itoa(page)}}
		var response envelope[episodesDTO]
		path := "/series/" + strconv.FormatInt(providerID, 10) + "/episodes/default/eng"
		if err := client.get(ctx, path, parameters, &response); err != nil {
			return nil, err
		}
		result = append(result, response.Data.Episodes...)
		if !hasNext(response.Links.Next) {
			return result, nil
		}
	}
	return nil, errors.New("TVDB episode response exceeded the page limit")
}

func (client *Client) get(ctx context.Context, path string, parameters url.Values, destination any) error {
	token, err := client.accessToken(ctx)
	if err != nil {
		return err
	}
	target := client.apiURL + path
	if len(parameters) > 0 {
		target += "?" + parameters.Encode()
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)
	if err := client.http.JSON(ctx, http.MethodGet, target, headers, nil, destination, true); err != nil {
		return fmt.Errorf("TVDB request failed: %w", err)
	}
	return nil
}

func (client *Client) accessToken(ctx context.Context) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.token != "" {
		return client.token, nil
	}
	if client.apiKey == "" {
		return "", errors.New("TVDB API key is not configured")
	}
	payload := map[string]string{"apikey": client.apiKey}
	if client.pin != "" {
		payload["pin"] = client.pin
	}
	encoded, _ := json.Marshal(payload)
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	var response envelope[struct {
		Token string `json:"token"`
	}]
	if err := client.http.JSON(ctx, http.MethodPost, client.apiURL+"/login", headers, encoded, &response, false); err != nil {
		return "", fmt.Errorf("TVDB authentication failed: %w", err)
	}
	if response.Data.Token == "" {
		return "", errors.New("TVDB authentication returned invalid data")
	}
	client.token = response.Data.Token
	return client.token, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func parseDate(value string) *time.Time {
	if len(value) < 10 {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value[:10])
	if err != nil {
		return nil
	}
	return &parsed
}
func parseYear(value string) *int {
	if len(value) >= 4 {
		parsed, err := strconv.Atoi(value[:4])
		if err == nil && parsed >= 1800 && parsed <= 3000 {
			return &parsed
		}
	}
	return nil
}
func hasNext(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != `""` && trimmed != "0"
}
func safeImage(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}
func names(values []namedDTO) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if name := strings.TrimSpace(value.Name); name != "" {
			result = append(result, name)
		}
	}
	return result
}
func network(row showDTO) string {
	if row.LatestNetwork != nil && row.LatestNetwork.Name != "" {
		return row.LatestNetwork.Name
	}
	if row.OriginalNetwork != nil {
		return row.OriginalNetwork.Name
	}
	for _, company := range row.Companies {
		if strings.Contains(strings.ToLower(company.CompanyType.CompanyTypeName), "network") {
			return company.Name
		}
	}
	return ""
}
func isDefaultSeason(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || strings.Contains(value, "aired") || strings.Contains(value, "default")
}
func syntheticSeasonID(showID int64, number int32) int64 { return showID*1000 + int64(number) + 1 }

func cast(values []characterDTO) []Person {
	sort.SliceStable(values, func(i, j int) bool { return values[i].Sort < values[j].Sort })
	result := make([]Person, 0, 10)
	for _, value := range values {
		if len(result) == 10 {
			break
		}
		name := strings.TrimSpace(value.PersonName)
		if name == "" {
			continue
		}
		result = append(result, Person{Name: name, Role: strings.TrimSpace(value.Name), ImageURL: safeImage(value.Image)})
	}
	return result
}
func artworks(values []artworkDTO) []media.Artwork {
	result := make([]media.Artwork, 0, len(values))
	preferred := map[string]bool{}
	for _, value := range values {
		kind := ""
		switch value.Type {
		case 2:
			kind = "poster"
		case 3:
			kind = "backdrop"
		case 8:
			kind = "logo"
		default:
			continue
		}
		image := safeImage(value.Image)
		if image == "" {
			continue
		}
		thumb := safeImage(value.Thumbnail)
		if thumb == "" {
			thumb = image
		}
		id := strconv.FormatInt(value.ID, 10)
		item := media.Artwork{ProviderImageID: id, Kind: kind, Language: value.Language, ImageURL: image, ThumbnailURL: thumb, Width: value.Width, Height: value.Height, Preferred: !preferred[kind], Available: true}
		preferred[kind] = true
		result = append(result, item)
	}
	return result
}
