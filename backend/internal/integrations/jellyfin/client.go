package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
)

const (
	pageSize       = 100
	maximumPages   = 100
	clientIdentity = `MediaBrowser Client="Gradeium", Device="Gradeium", DeviceId="gradeium", Version="1.1.0"`
)

type MediaType string

const (
	Movies  MediaType = "movies"
	TVShows MediaType = "tv"
)

type LibraryMapping struct {
	LibraryID string    `json:"libraryId"`
	Domain    MediaType `json:"domain"`
}

type Library struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CollectionType string    `json:"collectionType,omitempty"`
	Domain         MediaType `json:"domain,omitempty"`
}

type Item struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	ProviderID int64  `json:"providerId,omitempty"`
}

type Client struct {
	http    *provider.Client
	baseURL string
	apiKey  string
}

func NewClient(baseURL, apiKey string) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   provider.DefaultTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("Jellyfin redirects are not followed")
		},
	}
	return newClientWithHTTP(provider.NewClientWithHTTP(httpClient), baseURL, apiKey)
}

func newClientWithHTTP(httpClient *provider.Client, baseURL, apiKey string) (*Client, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("Jellyfin API key is required")
	}
	return &Client{http: httpClient, baseURL: normalized, apiKey: apiKey}, nil
}

func NormalizeBaseURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("Jellyfin server URL must be a valid HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Jellyfin server URL must not contain credentials, a query, or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (client *Client) Test(ctx context.Context) error {
	var response struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
	}
	if err := client.get(ctx, "/System/Info", nil, &response); err != nil {
		return err
	}
	if strings.TrimSpace(response.Version) == "" {
		return fmt.Errorf("%w: Jellyfin system information is incomplete", provider.ErrInvalidData)
	}
	return nil
}

func (client *Client) Libraries(ctx context.Context) ([]Library, error) {
	var response []struct {
		Name           string `json:"Name"`
		CollectionType string `json:"CollectionType"`
		ItemID         string `json:"ItemId"`
	}
	if err := client.get(ctx, "/Library/VirtualFolders", nil, &response); err != nil {
		return nil, err
	}
	result := make([]Library, 0, len(response))
	for _, value := range response {
		id, name := strings.TrimSpace(value.ItemID), strings.TrimSpace(value.Name)
		if id == "" || name == "" {
			continue
		}
		result = append(result, Library{ID: id, Name: name, CollectionType: strings.TrimSpace(value.CollectionType)})
	}
	return result, nil
}

func (client *Client) Items(ctx context.Context, libraryID string, mediaType MediaType) ([]Item, error) {
	includeType := "Movie"
	providerKey := "Tmdb"
	if mediaType == TVShows {
		includeType, providerKey = "Series", "Tvdb"
	} else if mediaType != Movies {
		return nil, errors.New("Jellyfin media type is not supported")
	}
	result := []Item{}
	for page := 0; page < maximumPages; page++ {
		query := url.Values{
			"ParentId":         {strings.TrimSpace(libraryID)},
			"IncludeItemTypes": {includeType},
			"Recursive":        {"true"},
			"Fields":           {"ProviderIds"},
			"StartIndex":       {strconv.Itoa(page * pageSize)},
			"Limit":            {strconv.Itoa(pageSize)},
		}
		var response struct {
			Items []struct {
				ID          string            `json:"Id"`
				Name        string            `json:"Name"`
				ProviderIDs map[string]string `json:"ProviderIds"`
			} `json:"Items"`
			TotalRecordCount int `json:"TotalRecordCount"`
		}
		if err := client.get(ctx, "/Items", query, &response); err != nil {
			return nil, err
		}
		for _, value := range response.Items {
			providerID := int64(0)
			for key, raw := range value.ProviderIDs {
				if strings.EqualFold(key, providerKey) {
					providerID, _ = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
					if providerID < 1 {
						providerID = 0
					}
					break
				}
			}
			result = append(result, Item{ID: strings.TrimSpace(value.ID), Title: strings.TrimSpace(value.Name), ProviderID: providerID})
		}
		if len(response.Items) < pageSize || (response.TotalRecordCount > 0 && len(result) >= response.TotalRecordCount) {
			return result, nil
		}
	}
	return nil, fmt.Errorf("%w: Jellyfin result set exceeds the pagination limit", provider.ErrInvalidData)
}

func (client *Client) get(ctx context.Context, endpoint string, query url.Values, destination any) error {
	target, err := url.Parse(client.baseURL)
	if err != nil {
		return err
	}
	target.Path = path.Join(target.Path, endpoint)
	target.RawQuery = query.Encode()
	headers := http.Header{}
	headers.Set("Authorization", clientIdentity+`, Token="`+client.apiKey+`"`)
	headers.Set("X-Emby-Token", client.apiKey)
	return client.http.JSON(ctx, http.MethodGet, target.String(), headers, nil, destination, true)
}
