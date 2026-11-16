package server

import (
	"net"
	"net/http"
)

// Guard wraps h so a web page the user merely visits cannot drive gebed.
//
// Two checks, outermost first:
//
//   - A Host allowlist. gebed listens on loopback, so a request whose Host is
//     not a loopback name (or the address gebed was told to listen on) is a
//     DNS-rebinding attempt and is refused. This is what CrossOriginProtection
//     does not cover, and it is what lets a rebinding domain read every bead.
//
//   - Go's http.CrossOriginProtection, which rejects unsafe-method
//     (POST/PUT/PATCH/DELETE) cross-origin requests using Sec-Fetch-Site and
//     Origin. Safe methods pass, so the read API is unaffected; the write
//     endpoints under /api/.../ops and DELETE /api/repos/{name} are covered.
//
// listenAddr is the value passed to -addr.
func Guard(listenAddr string, h http.Handler) http.Handler {
	h = http.NewCrossOriginProtection().Handler(h)
	allowed := loopbackHost(listenAddr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// loopbackHost reports whether a request's Host header may reach gebed: a
// loopback name, or the host:port gebed listens on. A wildcard bind
// (0.0.0.0, ::, or no host) adds nothing, so only loopback names are accepted
// then; expose gebed deliberately and it needs its own authentication anyway.
func loopbackHost(listenAddr string) func(string) bool {
	allow := map[string]struct{}{"localhost": {}, "127.0.0.1": {}, "::1": {}}
	var port string
	if h, p, err := net.SplitHostPort(listenAddr); err == nil {
		port = p
		switch h {
		case "", "0.0.0.0", "::":
		default:
			allow[h] = struct{}{}
		}
	}
	return func(host string) bool {
		if host == "" {
			return false
		}
		h, p, err := net.SplitHostPort(host)
		if err != nil { // Host carried no port
			h, p = host, ""
		}
		if port != "" && p != "" && p != port {
			return false
		}
		_, ok := allow[h]
		return ok
	}
}
