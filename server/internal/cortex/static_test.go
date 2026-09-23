package cortex

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPADeepLinksAndAPINotFound(t *testing.T) {
	handler := spaHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<html>cortex</html>")},
		"assets/app.js": {Data: []byte("console.log('cortex')")},
	})
	for _, test := range []struct {
		path   string
		status int
		body   string
	}{
		{"/", http.StatusOK, "<html>cortex</html>"},
		{"/project/42/jobs", http.StatusOK, "<html>cortex</html>"},
		{"/project/42/jobs/example/story", http.StatusOK, "<html>cortex</html>"},
		{"/assets/app.js", http.StatusOK, "console.log('cortex')"},
		{"/api/missing", http.StatusNotFound, "404 page not found\n"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status || response.Body.String() != test.body {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}
