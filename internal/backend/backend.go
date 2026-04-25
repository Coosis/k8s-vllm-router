package backend

import (
	"context"
	"net/http"
)

type Endpoint struct {
	ID  string
	URL string
}

type Client interface {
	Health(ctx context.Context, endpoint Endpoint) error
	Forward(w http.ResponseWriter, r *http.Request, endpoint Endpoint) error
}
