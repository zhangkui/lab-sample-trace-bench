package api

import (
	"encoding/json"
	"github.com/zhangkui/lab-sample-trace-bench/internal/domain"
	"github.com/zhangkui/lab-sample-trace-bench/internal/service"
	"net/http"
	"strings"
)

type Server struct{ app *service.Service }

func New(app *service.Service) *Server { return &Server{app: app} }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/containers", s.containers)
	mux.HandleFunc("/containers/", s.container)
	return mux
}
func health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
func (s *Server) containers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var c domain.Container
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out, err := s.app.RegisterContainer(c, r.Header.Get("X-Actor"))
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	write(w, 201, out)
}
func (s *Server) container(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id := parts[1]
	if len(parts) == 2 && r.Method == http.MethodGet {
		out, err := s.app.InspectContainer(id)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		write(w, 200, out)
		return
	}
	if len(parts) == 3 && parts[2] == "moves" && r.Method == http.MethodPost {
		var req service.MoveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		req.Container = id
		req.Operator = r.Header.Get("X-Actor")
		out, err := s.app.ApplyMove(req)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		write(w, 200, out)
		return
	}
	if len(parts) == 3 && parts[2] == "history" && r.Method == http.MethodGet {
		out, err := s.app.MoveHistory(id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		write(w, 200, out)
		return
	}
	http.NotFound(w, r)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
