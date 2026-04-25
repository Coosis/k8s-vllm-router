package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *HTTPClient) Health(ctx context.Context, endpoint Endpoint) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("backend %s health returned %d", endpoint.ID, resp.StatusCode)
	}
	return nil
}

func (c *HTTPClient) Forward(w http.ResponseWriter, r *http.Request, endpoint Endpoint) error {
	target, err := url.Parse(endpoint.URL)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
	return nil
}
