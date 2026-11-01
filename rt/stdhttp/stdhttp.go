// Package stdhttp exposes HTTP effects to gofx programs.
package stdhttp

import (
	"context"
	"net/http"
	"time"

	"gofx/rt"
)

var client = &http.Client{Timeout: 10 * time.Second}

// GetStatus constructs a deferred, cancellable HTTP GET. HTTP error statuses
// are values; transport failures are IO errors.
func GetStatus(url string) rt.IO {
	return func(ctx context.Context) (any, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		return int64(response.StatusCode), nil
	}
}
