// Package stdhttp exposes HTTP effects to gofx programs.
package stdhttp

import (
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}

// GetStatus fetches a URL and returns its HTTP status. HTTP error statuses are
// values; network failures are errors. The generated extern adapter supplies IO.
func GetStatus(url string) (int64, error) {
	response, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return int64(response.StatusCode), nil
}
