package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type SendFunc func(ctx context.Context, addr string, to string, text string) (AckResult, error)
type ResolveFunc func(alias string) (ResolveResult, error)
type StatusFunc func() any

type Service struct {
	Token   string
	Send    SendFunc
	Resolve ResolveFunc
	Status  StatusFunc
}

type AckResult struct {
	Status    string
	ErrorCode string
}

type ResolveResult struct {
	Found bool
	Entry any
}

type errorResp struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message,omitempty"`
}

type sendRequest struct {
	To   string `json:"to"`
	Text string `json:"text"`
	Addr string `json:"addr"`
}

type resolveRequest struct {
	Alias string `json:"alias"`
}

func NewHandler(svc Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", withAuth(svc.Token, statusHandler(svc)))
	mux.HandleFunc("/send", withAuth(svc.Token, sendHandler(svc)))
	mux.HandleFunc("/resolve", withAuth(svc.Token, resolveHandler(svc)))
	return mux
}

func withAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		prefix := "Bearer "
		if !strings.HasPrefix(auth, prefix) || strings.TrimPrefix(auth, prefix) != token {
			writeError(w, http.StatusUnauthorized, "ERR_GATEWAY_UNAUTHORIZED", "")
			return
		}
		next(w, r)
	}
}

func statusHandler(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "method")
			return
		}
		if svc.Status == nil {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "status")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(svc.Status())
	}
}

func sendHandler(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "method")
			return
		}
		var req sendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "json")
			return
		}
		if req.To == "" || req.Text == "" || req.Addr == "" {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "missing fields")
			return
		}
		if svc.Send == nil {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "send")
			return
		}
		res, err := svc.Send(r.Context(), req.Addr, req.To, req.Text)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":     res.Status,
			"error_code": res.ErrorCode,
		})
	}
}

func resolveHandler(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "method")
			return
		}
		var req resolveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "json")
			return
		}
		if req.Alias == "" {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "missing alias")
			return
		}
		if svc.Resolve == nil {
			writeError(w, http.StatusBadRequest, "ERR_GATEWAY_BAD_REQUEST", "resolve")
			return
		}
		res, err := svc.Resolve(req.Alias)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "")
			return
		}
		if !res.Found {
			writeError(w, http.StatusBadRequest, "ERR_ALIAS_NOT_FOUND", "")
			return
		}
		_ = json.NewEncoder(w).Encode(res.Entry)
	}
}

func writeError(w http.ResponseWriter, status int, code string, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResp{ErrorCode: code, Message: msg})
}

var ErrBadRequest = errors.New("ERR_GATEWAY_BAD_REQUEST")

var _ = ErrBadRequest
