package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	DefaultTimeout  = 8 * time.Second
	MaxResponseBody = 2 << 20
)

var (
	ErrUnavailable = errors.New("provider unavailable")
	ErrInvalidData = errors.New("provider returned invalid data")
)

type Client struct {
	httpClient *http.Client
	maxBody    int64
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 3 * time.Second
	transport.ResponseHeaderTimeout = 5 * time.Second
	transport.IdleConnTimeout = 60 * time.Second
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: DefaultTimeout}, maxBody: MaxResponseBody}
}

func NewClientWithHTTP(client *http.Client) *Client {
	return &Client{httpClient: client, maxBody: MaxResponseBody}
}

func (client *Client) JSON(
	ctx context.Context,
	method, target string,
	headers http.Header,
	body []byte,
	destination any,
	retrySafe bool,
) error {
	attempts := 1
	if retrySafe {
		attempts = 2
	}
	for attempt := 0; attempt < attempts; attempt++ {
		err, transient := client.jsonOnce(ctx, method, target, headers, body, destination)
		if err == nil {
			return nil
		}
		if !transient || attempt+1 == attempts {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%w: %v", ErrUnavailable, ctx.Err())
		case <-timer.C:
		}
	}
	return ErrUnavailable
}

func (client *Client) jsonOnce(
	ctx context.Context,
	method, target string,
	headers http.Header,
	body []byte,
	destination any,
) (error, bool) {
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: invalid endpoint", ErrUnavailable), false
	}
	request.Header = headers.Clone()
	request.Header.Set("Accept", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrUnavailable), true
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, client.maxBody+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("%w: response read failed", ErrUnavailable), true
	}
	if int64(len(payload)) > client.maxBody {
		return fmt.Errorf("%w: response too large", ErrInvalidData), false
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		transient := response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout
		return fmt.Errorf("%w: status %d", ErrUnavailable, response.StatusCode), transient
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: malformed JSON", ErrInvalidData), false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%w: trailing JSON data", ErrInvalidData), false
	}
	return nil, false
}
