package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
	"github.com/nzagler/gradeium/backend/internal/media"
)

const defaultAPIURL = "https://api.themoviedb.org/3"

type Client struct {
	http        *provider.Client
	accessToken string
	apiURL      string
}

func NewClient(accessToken string) *Client {
	return NewClientWithEndpoint(provider.NewClient(), accessToken, defaultAPIURL)
}

func NewClientWithEndpoint(httpClient *provider.Client, accessToken, apiURL string) *Client {
	return &Client{http: httpClient, accessToken: strings.TrimSpace(accessToken), apiURL: strings.TrimRight(apiURL, "/")}
}

type SearchResult struct {
	ProviderID int64  `json:"providerId"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	Director   string `json:"director,omitempty"`
	ArtworkURL string `json:"artworkUrl,omitempty"`
}

type SearchPage struct {
	Results []SearchResult `json:"results"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
}

type Person struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	ProfileURL string `json:"profileUrl,omitempty"`
}

type CollectionMember struct {
	ProviderID  int64      `json:"providerId"`
	Title       string     `json:"title"`
	ReleaseDate *time.Time `json:"releaseDate,omitempty"`
	PosterURL   string     `json:"posterUrl,omitempty"`
}

type Movie struct {
	ProviderID           int64              `json:"providerId"`
	Title                string             `json:"title"`
	OriginalTitle        string             `json:"originalTitle,omitempty"`
	Overview             string             `json:"overview,omitempty"`
	ReleaseDate          *time.Time         `json:"releaseDate,omitempty"`
	Year                 *int               `json:"year,omitempty"`
	RuntimeMinutes       *int32             `json:"runtimeMinutes,omitempty"`
	Director             string             `json:"director,omitempty"`
	Genres               []string           `json:"genres"`
	ProductionCompanies  []string           `json:"productionCompanies"`
	Cast                 []Person           `json:"cast"`
	Crew                 []Person           `json:"crew"`
	TrailerKey           string             `json:"trailerKey,omitempty"`
	IMDbID               string             `json:"imdbId,omitempty"`
	Homepage             string             `json:"homepage,omitempty"`
	CollectionID         *int64             `json:"collectionId,omitempty"`
	CollectionName       string             `json:"collectionName,omitempty"`
	Collection           []CollectionMember `json:"collection"`
	CommunityRating      *int16             `json:"communityRating,omitempty"`
	CommunityRatingCount *int32             `json:"communityRatingCount,omitempty"`
	Artworks             []media.Artwork    `json:"artworks"`
}

type VerifiedTV struct {
	TMDBID               int64
	CommunityRating      *int16
	CommunityRatingCount *int32
}

type searchResponse struct {
	Page       int              `json:"page"`
	TotalPages int              `json:"total_pages"`
	Results    []movieSearchDTO `json:"results"`
}
type movieSearchDTO struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
	PosterPath  string `json:"poster_path"`
}
type namedDTO struct {
	Name string `json:"name"`
}
type collectionDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type personDTO struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	Job         string `json:"job"`
	Department  string `json:"department"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}
type imageDTO struct {
	FilePath    string  `json:"file_path"`
	ISO6391     *string `json:"iso_639_1"`
	Width       int32   `json:"width"`
	Height      int32   `json:"height"`
	VoteAverage float64 `json:"vote_average"`
}
type videoDTO struct {
	Key      string `json:"key"`
	Site     string `json:"site"`
	Type     string `json:"type"`
	Official bool   `json:"official"`
	ISO6391  string `json:"iso_639_1"`
}
type movieDTO struct {
	ID                  int64          `json:"id"`
	Title               string         `json:"title"`
	OriginalTitle       string         `json:"original_title"`
	Overview            string         `json:"overview"`
	ReleaseDate         string         `json:"release_date"`
	Runtime             int32          `json:"runtime"`
	VoteAverage         float64        `json:"vote_average"`
	VoteCount           int32          `json:"vote_count"`
	Genres              []namedDTO     `json:"genres"`
	ProductionCompanies []namedDTO     `json:"production_companies"`
	BelongsToCollection *collectionDTO `json:"belongs_to_collection"`
	IMDbID              string         `json:"imdb_id"`
	Homepage            string         `json:"homepage"`
	PosterPath          string         `json:"poster_path"`
	BackdropPath        string         `json:"backdrop_path"`
	Credits             struct {
		Cast []personDTO `json:"cast"`
		Crew []personDTO `json:"crew"`
	} `json:"credits"`
	Images struct {
		Posters   []imageDTO `json:"posters"`
		Backdrops []imageDTO `json:"backdrops"`
		Logos     []imageDTO `json:"logos"`
	} `json:"images"`
	Videos struct {
		Results []videoDTO `json:"results"`
	} `json:"videos"`
}
type collectionResponse struct {
	Parts []movieSearchDTO `json:"parts"`
}

func (client *Client) Test(ctx context.Context) error {
	var response struct {
		Success bool `json:"success"`
	}
	return client.get(ctx, "/authentication", nil, &response)
}

func (client *Client) SearchMovies(ctx context.Context, query string, page int) (SearchPage, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return SearchPage{}, errors.New("search query must contain at least 2 characters")
	}
	if page < 1 || page > 500 {
		return SearchPage{}, errors.New("search page is out of range")
	}
	parameters := url.Values{"query": {query}, "page": {strconv.Itoa(page)}, "include_adult": {"false"}, "language": {"en-US"}}
	var response searchResponse
	if err := client.get(ctx, "/search/movie", parameters, &response); err != nil {
		return SearchPage{}, err
	}
	results := make([]SearchResult, 0, len(response.Results))
	for _, row := range response.Results {
		if row.ID <= 0 || strings.TrimSpace(row.Title) == "" {
			continue
		}
		item := SearchResult{ProviderID: row.ID, Title: row.Title, Year: year(row.ReleaseDate)}
		if row.PosterPath != "" {
			item.ArtworkURL = imageURL("w342", row.PosterPath)
		}
		results = append(results, item)
	}
	return SearchPage{Results: results, Page: page, HasMore: response.Page < response.TotalPages}, nil
}

func (client *Client) Movie(ctx context.Context, providerID int64) (Movie, error) {
	if providerID <= 0 {
		return Movie{}, errors.New("invalid TMDB movie ID")
	}
	parameters := url.Values{
		"language":               {"en-US"},
		"append_to_response":     {"credits,images,videos"},
		"include_image_language": {"en,null"},
	}
	var row movieDTO
	if err := client.get(ctx, "/movie/"+strconv.FormatInt(providerID, 10), parameters, &row); err != nil {
		return Movie{}, err
	}
	if row.ID != providerID || strings.TrimSpace(row.Title) == "" {
		return Movie{}, errors.New("TMDB returned invalid movie data")
	}
	director, cast, crew := people(row.Credits.Cast, row.Credits.Crew)
	movie := Movie{
		ProviderID: row.ID, Title: row.Title, OriginalTitle: row.OriginalTitle, Overview: strings.TrimSpace(row.Overview),
		ReleaseDate: parseDate(row.ReleaseDate), Year: year(row.ReleaseDate), Director: director,
		Genres: names(row.Genres), ProductionCompanies: names(row.ProductionCompanies), Cast: cast, Crew: crew,
		IMDbID: strings.TrimSpace(row.IMDbID), Homepage: safeURL(row.Homepage), Collection: make([]CollectionMember, 0),
		CommunityRating: media.CommunityRating(row.VoteAverage, 10), Artworks: artworks(row), TrailerKey: trailer(row.Videos.Results),
	}
	if row.Runtime > 0 {
		runtime := row.Runtime
		movie.RuntimeMinutes = &runtime
	}
	if row.VoteCount > 0 {
		count := row.VoteCount
		movie.CommunityRatingCount = &count
	}
	if row.BelongsToCollection != nil && row.BelongsToCollection.ID > 0 {
		id := row.BelongsToCollection.ID
		movie.CollectionID = &id
		movie.CollectionName = row.BelongsToCollection.Name
		var collection collectionResponse
		if err := client.get(ctx, "/collection/"+strconv.FormatInt(id, 10), url.Values{"language": {"en-US"}}, &collection); err == nil {
			for _, part := range collection.Parts {
				member := CollectionMember{ProviderID: part.ID, Title: part.Title, ReleaseDate: parseDate(part.ReleaseDate)}
				if part.PosterPath != "" {
					member.PosterURL = imageURL("w342", part.PosterPath)
				}
				movie.Collection = append(movie.Collection, member)
			}
			sort.SliceStable(movie.Collection, func(i, j int) bool {
				left, right := movie.Collection[i].ReleaseDate, movie.Collection[j].ReleaseDate
				if left == nil {
					return false
				}
				if right == nil {
					return true
				}
				return left.Before(*right)
			})
		}
	}
	return movie, nil
}

func (client *Client) VerifyTVDBMapping(ctx context.Context, tvdbID int64) (*VerifiedTV, error) {
	if tvdbID <= 0 {
		return nil, errors.New("invalid TVDB ID")
	}
	var found struct {
		TVResults []struct {
			ID int64 `json:"id"`
		} `json:"tv_results"`
	}
	parameters := url.Values{"external_source": {"tvdb_id"}, "language": {"en-US"}}
	if err := client.get(ctx, "/find/"+strconv.FormatInt(tvdbID, 10), parameters, &found); err != nil {
		return nil, err
	}
	if len(found.TVResults) != 1 || found.TVResults[0].ID <= 0 {
		return nil, nil
	}
	tmdbID := found.TVResults[0].ID
	var external struct {
		TVDBID int64 `json:"tvdb_id"`
	}
	if err := client.get(ctx, "/tv/"+strconv.FormatInt(tmdbID, 10)+"/external_ids", nil, &external); err != nil {
		return nil, err
	}
	if external.TVDBID != tvdbID {
		return nil, nil
	}
	var details struct {
		ID          int64   `json:"id"`
		VoteAverage float64 `json:"vote_average"`
		VoteCount   int32   `json:"vote_count"`
	}
	if err := client.get(ctx, "/tv/"+strconv.FormatInt(tmdbID, 10), url.Values{"language": {"en-US"}}, &details); err != nil {
		return nil, err
	}
	if details.ID != tmdbID {
		return nil, nil
	}
	result := &VerifiedTV{TMDBID: tmdbID, CommunityRating: media.CommunityRating(details.VoteAverage, 10)}
	if details.VoteCount > 0 {
		count := details.VoteCount
		result.CommunityRatingCount = &count
	}
	return result, nil
}

func (client *Client) get(ctx context.Context, path string, parameters url.Values, destination any) error {
	if client.accessToken == "" {
		return errors.New("TMDB access token is not configured")
	}
	target := client.apiURL + path
	if len(parameters) > 0 {
		target += "?" + parameters.Encode()
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+client.accessToken)
	if err := client.http.JSON(ctx, http.MethodGet, target, headers, nil, destination, true); err != nil {
		return fmt.Errorf("TMDB request failed: %w", err)
	}
	return nil
}

func parseDate(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}
func year(value string) *int {
	parsed := parseDate(value)
	if parsed == nil {
		return nil
	}
	result := parsed.Year()
	return &result
}
func imageURL(size, path string) string {
	return "https://image.tmdb.org/t/p/" + size + "/" + strings.TrimPrefix(path, "/")
}
func safeURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
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

func people(castRows, crewRows []personDTO) (string, []Person, []Person) {
	sort.SliceStable(castRows, func(i, j int) bool { return castRows[i].Order < castRows[j].Order })
	cast := make([]Person, 0, 10)
	for _, row := range castRows {
		if len(cast) == 10 {
			break
		}
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		cast = append(cast, Person{Name: row.Name, Role: row.Character, ProfileURL: profileURL(row.ProfilePath)})
	}
	director := ""
	crew := make([]Person, 0, 8)
	seen := map[string]struct{}{}
	for _, row := range crewRows {
		role := ""
		switch strings.ToLower(row.Job) {
		case "director":
			role = "Director"
		case "screenplay", "writer":
			role = "Writing"
		case "director of photography":
			role = "Cinematography"
		case "original music composer":
			role = "Music"
		default:
			continue
		}
		if role == "Director" && director == "" {
			director = row.Name
		}
		key := role + "\x00" + row.Name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(crew) < 8 {
			crew = append(crew, Person{Name: row.Name, Role: role, ProfileURL: profileURL(row.ProfilePath)})
		}
	}
	return director, cast, crew
}
func profileURL(path string) string {
	if path == "" {
		return ""
	}
	return imageURL("w185", path)
}

func artworks(row movieDTO) []media.Artwork {
	result := make([]media.Artwork, 0, len(row.Images.Posters)+len(row.Images.Backdrops)+len(row.Images.Logos)+2)
	add := func(image imageDTO, kind string, preferred bool) {
		if image.FilePath == "" {
			return
		}
		language := ""
		if image.ISO6391 != nil {
			language = *image.ISO6391
		}
		result = append(result, media.Artwork{ProviderImageID: image.FilePath, Kind: kind, Language: language, ImageURL: imageURL("original", image.FilePath), ThumbnailURL: imageURL("w342", image.FilePath), Width: image.Width, Height: image.Height, Preferred: preferred, Available: true})
	}
	seen := map[string]struct{}{}
	groups := []struct {
		rows                []imageDTO
		kind, preferredPath string
	}{{row.Images.Posters, "poster", row.PosterPath}, {row.Images.Backdrops, "backdrop", row.BackdropPath}, {row.Images.Logos, "logo", ""}}
	for _, group := range groups {
		if group.preferredPath != "" {
			add(imageDTO{FilePath: group.preferredPath}, group.kind, true)
			seen[group.preferredPath] = struct{}{}
		}
		for index, image := range group.rows {
			if _, ok := seen[image.FilePath]; ok {
				continue
			}
			add(image, group.kind, group.preferredPath == "" && index == 0)
			seen[image.FilePath] = struct{}{}
		}
	}
	return result
}

func trailer(videos []videoDTO) string {
	for _, video := range videos {
		if strings.EqualFold(video.Site, "YouTube") && strings.EqualFold(video.Type, "Trailer") && video.Official && (video.ISO6391 == "en" || video.ISO6391 == "") {
			return video.Key
		}
	}
	for _, video := range videos {
		if strings.EqualFold(video.Site, "YouTube") && strings.EqualFold(video.Type, "Trailer") {
			return video.Key
		}
	}
	return ""
}
