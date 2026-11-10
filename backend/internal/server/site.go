package server

import "net/http"

// What the server answers outside /api/: GET /healthz, and the SPA for every other
// path. The API's own routes are the operations in internal/api.

// siteRoutes builds siteMux. ServeHTTP hands it every request outside /api/, ahead of
// the API's session, CSRF and access-log layers.
func (s *Server) siteRoutes() {
	s.siteMux = http.NewServeMux()
	s.siteMux.HandleFunc("GET /healthz", s.handleHealth)
	s.siteMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if s.site == nil {
			http.NotFound(w, r)
			return
		}
		s.site.ServeHTTP(w, r)
	})
}

// SetSite gives the server the SPA to serve outside /api/ (internal/web's handler on
// the embedded build). Without one, those paths answer 404, as in the API's tests.
func (s *Server) SetSite(site http.Handler) {
	s.site = site
}
