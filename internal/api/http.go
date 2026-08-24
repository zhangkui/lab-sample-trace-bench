package api

import (
	"encoding/json"
	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/service"
	"net/http"
	"strings"
	"time"
)

type Server struct{ app *service.SampleService }

func New(app *service.SampleService) *Server { return &Server{app: app} }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/samples", s.samples)
	mux.HandleFunc("/samples/", s.sample)
	return mux
}
func (s *Server) health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
func (s *Server) samples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var sample domain.Sample
	if err := json.NewDecoder(r.Body).Decode(&sample); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	actor := r.Header.Get("X-Actor")
	out, err := s.app.Create(sample, actor)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	write(w, 201, out)
}
func (s *Server) sample(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	if len(parts) == 2 && r.Method == http.MethodGet {
		out, err := s.app.Get(id)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		write(w, 200, out)
		return
	}
	if len(parts) == 3 && parts[2] == "transition" && r.Method == http.MethodPost {
		var req struct {
			Status domain.Status `json:"status"`
			Note   string        `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		out, err := s.app.Transition(id, req.Status, r.Header.Get("X-Actor"), req.Note)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		write(w, 200, out)
		return
	}
	if len(parts) == 3 && parts[2] == "trace" && r.Method == http.MethodGet {
		write(w, 200, map[string]any{"sample_id": id, "at": time.Now().UTC()})
		return
	}
	http.NotFound(w, r)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
