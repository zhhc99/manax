package ipc

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server exposes the daemon handler over a local transport.
type Server struct {
	l    net.Listener
	http *http.Server
	once sync.Once
}

// NewServer binds the local transport and prepares the HTTP server.
func NewServer(h Handler) (*Server, error) {
	l, err := listen()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	route := func(path string, fn func(w http.ResponseWriter, r *http.Request)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fn(w, r)
		})
	}
	decode := func(w http.ResponseWriter, r *http.Request, out any) bool {
		if err := json.NewDecoder(r.Body).Decode(out); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(SimpleResponse{Err: "invalid request body"})
			return false
		}
		return true
	}

	route("/status", func(w http.ResponseWriter, r *http.Request) {
		var req StatusRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleStatus(req))
		}
	})
	route("/apply", func(w http.ResponseWriter, r *http.Request) {
		var req ApplyRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleApply(req))
		}
	})
	route("/mode", func(w http.ResponseWriter, r *http.Request) {
		var req ModeRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleMode(req))
		}
	})
	route("/system", func(w http.ResponseWriter, r *http.Request) {
		var req SystemRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleSystem(req))
		}
	})
	route("/tun", func(w http.ResponseWriter, r *http.Request) {
		var req TunRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleTun(req))
		}
	})
	route("/nodes", func(w http.ResponseWriter, r *http.Request) {
		var req NodesRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleNodes(req))
		}
	})
	route("/delay", func(w http.ResponseWriter, r *http.Request) {
		var req DelayRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleDelay(req))
		}
	})
	route("/select", func(w http.ResponseWriter, r *http.Request) {
		var req SelectRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleSelect(req))
		}
	})
	route("/stop", func(w http.ResponseWriter, r *http.Request) {
		var req StopRequest
		if decode(w, r, &req) {
			_ = json.NewEncoder(w).Encode(h.HandleStop(req))
		}
	})

	return &Server{l: l, http: &http.Server{Handler: mux, ReadHeaderTimeout: 30 * time.Second}}, nil
}

// Serve blocks until the server is closed.
func (s *Server) Serve() { _ = s.http.Serve(s.l) }

// Close stops the server.
func (s *Server) Close() {
	s.once.Do(func() { _ = s.http.Close() })
}
