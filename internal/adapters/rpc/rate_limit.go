package rpc

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"aim-chat/go-backend/internal/platform/ratelimiter"
)

const (
	rpcRateLimitEnabledEnv = "AIM_RPC_RATE_LIMIT_ENABLED"
	rpcRateLimitRPSEnv     = "AIM_RPC_RATE_LIMIT_RPS"
	rpcRateLimitBurstEnv   = "AIM_RPC_RATE_LIMIT_BURST"
)

type rpcRateLimitConfig struct {
	Enabled bool
	RPS     float64
	Burst   int
}

type rpcRateLimiter struct {
	limiter *ratelimiter.MapLimiter
}

func loadRPCRateLimitConfig() rpcRateLimitConfig {
	cfg := rpcRateLimitConfig{
		Enabled: true,
		RPS:     30,
		Burst:   60,
	}
	if env, ok := parseBoolEnv(rpcRateLimitEnabledEnv); ok {
		cfg.Enabled = env
	} else {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
		case "test", "testing":
			cfg.Enabled = false
		}
	}
	if raw := strings.TrimSpace(os.Getenv(rpcRateLimitRPSEnv)); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			cfg.RPS = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv(rpcRateLimitBurstEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.Burst = parsed
		}
	}
	return cfg
}

func newRPCRateLimiter(cfg rpcRateLimitConfig) *rpcRateLimiter {
	if !cfg.Enabled {
		return nil
	}
	return &rpcRateLimiter{
		limiter: ratelimiter.New(cfg.RPS, cfg.Burst, 10*time.Minute),
	}
}

func (l *rpcRateLimiter) allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	return l.limiter.Allow(key, now)
}

func rpcRateLimitKey(r *http.Request, token string) string {
	if strings.TrimSpace(token) != "" {
		return "token:" + token
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return "ip:unknown"
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return "ip:" + remote
	}
	if strings.TrimSpace(host) == "" {
		return "ip:unknown"
	}
	return "ip:" + host
}
