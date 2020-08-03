package context

import (
	"net/http"
)

func TenantIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tenantID string
		// check for basic auth header as first to gather the tenant ID. If not available use the header field for OrgID
		if user, _, ok := r.BasicAuth(); ok {
			tenantID = user
		} else if header := r.Header.Get("X-Scope-OrgID"); header != "" {
			tenantID = header
		}
		next.ServeHTTP(w, r.WithContext(ContextWithTenantID(r.Context(), tenantID)))
	})
}

func ClientIPHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(ContextWithClientIP(r.Context(), r.RemoteAddr)))
	})
}
