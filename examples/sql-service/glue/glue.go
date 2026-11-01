package glue

import (
	"fmt"
	"net/http/httptest"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "modernc.org/sqlite"
)

func RenderMetrics() (string, error) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost/metrics", nil)
	promhttp.Handler().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		return "", fmt.Errorf("promhttp: status %d: %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String(), nil
}
