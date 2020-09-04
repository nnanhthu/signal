package client

import (
	"github.com/nnanhthu/signal/api"
	"github.com/nnanhthu/signal/transports"
	"github.com/nnanhthu/signal/transports/http"
	"github.com/pkg/errors"
	"net/url"
	"time"
)

var (
	ErrInitializeTransport  = errors.New("Failed to initialize transport.")
	ErrUnsupportedTransport = errors.New("Unsupported transport.")
)

type Client struct {
	cc  transports.CallCloser
	API *api.API
}

// NewClient creates a new RPC client that use the given CallCloser internally.
// Initialize only server present API. Absent API initialized as nil value.
func NewClient(s, token string, timeout time.Duration) (*Client, error) {
	// Parse URL
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	// Initializing Transport
	var call transports.CallCloser
	switch u.Scheme {
	case "wss", "ws":
		return nil, ErrUnsupportedTransport
	case "https", "http":
		call, err = http.NewTransport(s, token, timeout)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInitializeTransport
	}
	client := &Client{cc: call}
	client.API = api.NewAPI(client.cc)
	return client, nil
}

// Close should be used to close the client when no longer needed.
// It simply calls Close() on the underlying CallCloser.
func (client *Client) Close() error {
	return client.cc.Close()
}
