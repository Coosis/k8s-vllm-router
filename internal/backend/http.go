package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type HTTPClient struct {
	client     *http.Client
	healthPath string
}

func NewHTTPClient(healthPath string) *HTTPClient {
	if healthPath == "" {
		healthPath = "/healthz"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	return &HTTPClient{
		client:     &http.Client{Timeout: 5 * time.Second},
		healthPath: healthPath,
	}
}

func (c *HTTPClient) Health(ctx context.Context, endpoint Endpoint) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL+c.healthPath, nil)
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

func (c *HTTPClient) Forward(w http.ResponseWriter, r *http.Request, endpoint Endpoint) (bool, error) {
	target, err := url.Parse(endpoint.URL)
	if err != nil {
		return false, err
	}
	var proxyErr error
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		proxyErr = err
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
	return true, proxyErr
}
