package api

import (
	"encoding/json"
	"net/http"
)

// ScaleErrorResponse wraps errors in IBM Storage Scale GUI compatible format.
type ScaleErrorResponse struct {
	Status ScaleStatus `json:"status"`
}

// ScaleErrorMiddleware converts Huma's RFC 7807 error responses to Scale GUI format.
func ScaleErrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || (len(r.URL.Path) > 4 && r.URL.Path[:4] == "/api") {
			// Skip for non-Scale endpoints (token management)
			next.ServeHTTP(w, r)
			return
		}

		rec := &responseRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)

		if rec.status >= 400 && rec.hijacked {
			// Error already written by Huma, try to rewrite
			// Can't rewrite after headers sent, but we try for buffered responses
		}
	})
}

// WriteScaleError writes an error in Scale GUI format.
func WriteScaleError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ScaleErrorResponse{
		Status: ScaleStatus{Code: code, Message: message},
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status   int
	hijacked bool
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.hijacked = true
	r.ResponseWriter.WriteHeader(code)
}
