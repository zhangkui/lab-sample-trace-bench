package handler

import (
	"encoding/json"
	"github.com/zhangkui/lab-sample-trace-bench/internal/service"
	"net/http"
	"strings"
	"time"
)

func New(app *service.Lab) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/records", records(app))
	mux.HandleFunc("/records/", records(app))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}
func records(app *service.Lab) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == http.MethodPost && r.URL.Path == "/records" {
			var rec service.Record
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if rec.CreatedAt.IsZero() {
				rec.CreatedAt = time.Now().UTC()
			}
			if err := app.Put(rec); err != nil {
				http.Error(w, err.Error(), 422)
				return
			}
			writeJSON(w, 201, rec)
			return
		}
		if len(parts) == 3 {
			kind, id := parts[1], parts[2]
			if r.Method == http.MethodGet {
				rec, err := app.Get(kind, id)
				if err != nil {
					http.Error(w, "not found", 404)
					return
				}
				writeJSON(w, 200, rec)
				return
			}
			if r.Method == http.MethodDelete {
				if err := app.Delete(kind, id); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.WriteHeader(204)
				return
			}
		}
		http.NotFound(w, r)
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
