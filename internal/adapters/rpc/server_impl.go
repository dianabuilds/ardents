package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
)

const DefaultRPCAddr = "127.0.0.1:8787"

type Server struct {
	httpServer    *http.Server
	service       contracts.DaemonService
	initErr       error
	rpcToken      string
	requireRPC    bool
	groupsEnabled bool
	rpcLimiter    *rpcRateLimiter
	streams       *rpcStreamLimiter
}

func NewServerWithService(rpcAddr string, svc contracts.DaemonService) *Server {
	requireRPC := requiresRPCToken()
	rpcToken, err := resolveRPCToken()
	if err != nil {
		return &Server{initErr: err}
	}
	if requireRPC && rpcToken == "" {
		return &Server{
			initErr: errors.New("AIM_RPC_TOKEN is required unless AIM_REQUIRE_RPC_TOKEN=false or AIM_ENV is test/development/local"),
		}
	}
	return newServerWithService(rpcAddr, svc, rpcToken, requireRPC)
}

func newServerWithService(rpcAddr string, svc contracts.DaemonService, rpcToken string, requireRPC bool) *Server {
	if rpcAddr == "" {
		rpcAddr = DefaultRPCAddr
	}

	mux := http.NewServeMux()
	s := &Server{
		httpServer: &http.Server{
			Addr:              rpcAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		service:       svc,
		rpcToken:      rpcToken,
		requireRPC:    requireRPC,
		groupsEnabled: groupsEnabled(),
		rpcLimiter:    newRPCRateLimiter(loadRPCRateLimitConfig()),
		streams:       newRPCStreamLimiter(loadRPCStreamLimitConfig()),
	}
	if s.rpcToken == "" && !s.requireRPC {
		slog.Default().Warn("AIM_RPC_TOKEN is not set; RPC auth disabled")
	}
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/rpc", s.handleRPC)
	mux.HandleFunc("/rpc/stream", s.handleRPCStream)
	mux.HandleFunc("/files/", s.handleFileDownload)
	return s
}

func (s *Server) Run(ctx context.Context) error {
	if s.initErr != nil {
		return s.initErr
	}
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	if err := s.service.StartNetworking(ctx); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			cancel()
			return err
		}
		if err := s.service.StopNetworking(shutdownCtx); err != nil {
			cancel()
			return err
		}
		cancel()
		return <-errCh
	case err := <-errCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.service.StopNetworking(shutdownCtx)
		cancel()
		return err
	}
}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	s.handleHealth(w, r)
}

func (s *Server) HandleRPC(w http.ResponseWriter, r *http.Request) {
	s.handleRPC(w, r)
}

func (s *Server) HandleRPCStream(w http.ResponseWriter, r *http.Request) {
	s.handleRPCStream(w, r)
}

func (s *Server) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	s.handleFileDownload(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !s.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleRPCStream(w http.ResponseWriter, r *http.Request) {
	if !s.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorizeRPC(w, r) {
		return
	}
	clientKey := rpcRateLimitKey(r, s.extractRPCToken(r))
	release, allowed := s.streams.acquire(clientKey)
	if !allowed {
		http.Error(w, "too many stream subscriptions", http.StatusTooManyRequests)
		return
	}
	defer release()
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	cursor := int64(0)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		cursor = v
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	replay, ch, cancel := s.service.SubscribeNotifications(cursor)
	defer cancel()

	for _, evt := range replay {
		if err := writeSSEEvent(w, evt); err != nil {
			return
		}
		flusher.Flush()
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, evt); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, evt NotificationEvent) error {
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  evt.Method,
		"params": map[string]any{
			"version":   1,
			"seq":       evt.Seq,
			"timestamp": evt.Timestamp,
			"payload":   evt.Payload,
		},
	}
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\n", evt.Seq); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
		return err
	}
	return nil
}

func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" && !isAllowedOrigin(origin) {
		http.Error(w, "origin is not allowed", http.StatusForbidden)
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-AIM-RPC-Token")
	return true
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if !s.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorizeRPC(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(path.Clean(r.URL.Path), "/files/")
	id = strings.TrimSpace(id)
	if id == "" || id == "." || strings.Contains(id, "/") {
		http.Error(w, "invalid attachment id", http.StatusBadRequest)
		return
	}

	meta, data, err := s.service.GetAttachment(id)
	if err != nil {
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}

	filename := meta.Name
	if filename == "" {
		filename = id
	}
	w.Header().Set("Content-Type", meta.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.QueryEscape(filename))
	_, _ = w.Write(data)
}

func (s *Server) authorizeRPC(w http.ResponseWriter, r *http.Request) bool {
	if s.rpcToken == "" && !s.requireRPC {
		return true
	}
	token := s.extractRPCToken(r)
	if token != s.rpcToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (s *Server) extractRPCToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get("X-AIM-RPC-Token"))
	if token != "" {
		return token
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func requiresRPCToken() bool {
	if v, ok := parseBoolEnv("AIM_REQUIRE_RPC_TOKEN"); ok {
		if !v && !isNonProdEnv() {
			// Fail-closed in production-like environments.
			return true
		}
		return v
	}
	if isNonProdEnv() {
		return false
	}
	return true
}

func isNonProdEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
	case "test", "testing", "dev", "development", "local":
		return true
	default:
		return false
	}
}

func isAllowedOrigin(raw string) bool {
	if raw == "null" {
		allowNull, _ := parseBoolEnv("AIM_ALLOW_NULL_ORIGIN")
		return allowNull
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return false
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func parseBoolEnv(name string) (bool, bool) {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func resolveRPCToken() (string, error) {
	token := strings.TrimSpace(os.Getenv("AIM_RPC_TOKEN"))
	rotate := strings.EqualFold(token, "auto")
	if !rotate {
		if v, ok := parseBoolEnv("AIM_RPC_TOKEN_ROTATE_ON_START"); ok && v {
			rotate = true
		}
	}
	if rotate {
		generated, err := generateRPCToken()
		if err != nil {
			return "", err
		}
		token = generated
		_ = os.Setenv("AIM_RPC_TOKEN", token)
		if err := persistRPCToken(token); err != nil {
			return "", err
		}
	}
	return token, nil
}

func generateRPCToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "rpc_" + hex.EncodeToString(buf), nil
}

func persistRPCToken(token string) error {
	pathValue := strings.TrimSpace(os.Getenv("AIM_RPC_TOKEN_FILE"))
	if pathValue == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(pathValue), 0o700); err != nil {
		return err
	}
	return os.WriteFile(pathValue, []byte(token), 0o600)
}

func groupsEnabled() bool {
	if v, ok := parseBoolEnv("AIM_GROUPS_ENABLED"); ok {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
	case "test", "testing", "dev", "development", "local":
		return true
	default:
		return false
	}
}
