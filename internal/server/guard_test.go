package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuard(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true })
	h := Guard("127.0.0.1:7878", inner)

	cases := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		want    int // want status; 0 means the inner handler must run (200)
	}{
		{"loopback GET reaches inner", "GET", "127.0.0.1:7878", nil, 0},
		{"localhost GET reaches inner", "GET", "localhost:7878", nil, 0},
		{"same-origin POST reaches inner", "POST", "127.0.0.1:7878",
			map[string]string{"Origin": "http://127.0.0.1:7878", "Sec-Fetch-Site": "same-origin"}, 0},
		{"non-browser POST reaches inner", "POST", "127.0.0.1:7878", nil, 0},
		{"cross-origin POST blocked", "POST", "127.0.0.1:7878",
			map[string]string{"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"cross-origin POST without Sec-Fetch-Site blocked", "POST", "127.0.0.1:7878",
			map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"rebinding Host blocked", "GET", "evil.example", nil, http.StatusForbidden},
		{"rebinding Host with port blocked", "POST", "evil.example:7878",
			map[string]string{"Origin": "http://evil.example:7878", "Sec-Fetch-Site": "same-origin"}, http.StatusForbidden},
		{"wrong port blocked", "GET", "127.0.0.1:9999", nil, http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reached = false
			r := httptest.NewRequest(c.method, "/api/repos", nil)
			r.Host = c.host
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if c.want == 0 {
				if !reached {
					t.Fatalf("inner handler not reached; status %d", w.Code)
				}
				return
			}
			if reached {
				t.Fatalf("inner handler ran but the request should have been refused")
			}
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}
