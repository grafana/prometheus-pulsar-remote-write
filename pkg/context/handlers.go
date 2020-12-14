package context

import (
	"net/http"
	"time"
)

const (
	HTTPHeaderTenantID = "X-Scope-OrgID"
)

type realClock struct {
}

func (_ *realClock) Now() time.Time {
	return time.Now()
}

type Clock interface {
	Now() time.Time
}

var clock Clock = &realClock{}

func WithClock(c Clock) {
	clock = c
}

func TenantIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tenantID string
		// check for basic auth header as first to gather the tenant ID. If not available use the header field for OrgID
		if user, _, ok := r.BasicAuth(); ok {
			tenantID = user
		} else if header := r.Header.Get(HTTPHeaderTenantID); header != "" {
			tenantID = header
		}
		next.ServeHTTP(w, r.WithContext(ContextWithTenantID(r.Context(), tenantID)))
	})
}

func MaxConnectionAgeHandler(next http.Handler, maxConnectionAge time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if start, ok := ConnectionStartTimeFromContext(r.Context()); ok && clock.Now().After(start.Add(maxConnectionAge)) {
			w.Header().Set("Connection", "close")
		}
		next.ServeHTTP(w, r)
	})
}
