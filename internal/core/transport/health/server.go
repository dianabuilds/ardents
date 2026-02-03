package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type StatusProvider interface {
	Status() (state string, peersConnected uint64)
}

type Server struct {
	srv *http.Server
}

func Start(ctx context.Context, addr string, provider StatusProvider) (*Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		state, peers := provider.Status()
		resp := map[string]any{
			"status":          state,
			"peers_connected": peers,
		}
		data, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "failed to encode health status", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(data); err != nil {
			return
		}
	})
	s := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdownCtx)
	}()
	return &Server{srv: s}, nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
