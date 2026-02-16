package rpc

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
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
	limit   rate.Limit
	burst   int
	mu      sync.Mutex
	byKey   map[string]*rpcRateLimitEntry
	hits    uint64
	idleTTL time.Duration
}

type rpcRateLimitEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
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
		limit:   rate.Limit(cfg.RPS),
		burst:   cfg.Burst,
		byKey:   make(map[string]*rpcRateLimitEntry),
		idleTTL: 10 * time.Minute,
	}
}

func (l *rpcRateLimiter) allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.byKey[key]
	if !ok {
		entry = &rpcRateLimitEntry{
			limiter:  rate.NewLimiter(l.limit, l.burst),
			lastSeen: now,
		}
		l.byKey[key] = entry
	}
	entry.lastSeen = now
	allowed := entry.limiter.AllowN(now, 1)

	l.hits++
	if l.hits%512 == 0 {
		cutoff := now.Add(-l.idleTTL)
		for k, v := range l.byKey {
			if v.lastSeen.Before(cutoff) {
				delete(l.byKey, k)
			}
		}
	}
	return allowed
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
