// Package stdhttp exposes HTTP effects to gofx programs.
package stdhttp

import (
	"net/http"
	"time"

	"gofx/rt"
)

var client = &http.Client{Timeout: 10 * time.Second}

// GetStatus constructs an IO that fetches a URL and returns its HTTP status.
// HTTP error statuses are values; network failures are IO errors.
func GetStatus(url any) any {
	return rt.IO(func() (any, error) {
		response, err := client.Get(url.(string))
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		return int64(response.StatusCode), nil
	})
}
