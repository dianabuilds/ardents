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
	rpcRateLimitEnabledEnv  = "AIM_RPC_RATE_LIMIT_ENABLED"
	rpcRateLimitRPSEnv      = "AIM_RPC_RATE_LIMIT_RPS"
	rpcRateLimitBurstEnv    = "AIM_RPC_RATE_LIMIT_BURST"
	fileRateLimitEnabledEnv = "AIM_FILE_RATE_LIMIT_ENABLED"
	fileRateLimitRPSEnv     = "AIM_FILE_RATE_LIMIT_RPS"
	fileRateLimitBurstEnv   = "AIM_FILE_RATE_LIMIT_BURST"
)

type rpcRateLimitConfig struct {
	Enabled bool
	RPS     float64
	Burst   int
}

type fileRateLimitConfig struct {
	Enabled bool
	RPS     float64
	Burst   int
}

type rpcRateLimiter struct {
	limiter *ratelimiter.MapLimiter
}

func loadRPCRateLimitConfig() rpcRateLimitConfig {
	enabled, rps, burst := loadRateLimitConfig(
		rpcRateLimitEnabledEnv,
		rpcRateLimitRPSEnv,
		rpcRateLimitBurstEnv,
		true,
		30,
		60,
	)
	return rpcRateLimitConfig{Enabled: enabled, RPS: rps, Burst: burst}
}

func loadFileRateLimitConfig() fileRateLimitConfig {
	enabled, rps, burst := loadRateLimitConfig(
		fileRateLimitEnabledEnv,
		fileRateLimitRPSEnv,
		fileRateLimitBurstEnv,
		true,
		12,
		24,
	)
	return fileRateLimitConfig{Enabled: enabled, RPS: rps, Burst: burst}
}

func loadRateLimitConfig(enabledEnv, rpsEnv, burstEnv string, defaultEnabled bool, defaultRPS float64, defaultBurst int) (bool, float64, int) {
	enabled := defaultEnabled
	if env, ok := parseBoolEnv(enabledEnv); ok {
		enabled = env
	} else {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("AIM_ENV"))) {
		case "test", "testing":
			enabled = false
		}
	}

	rps := defaultRPS
	if raw := strings.TrimSpace(os.Getenv(rpsEnv)); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed > 0 {
			rps = parsed
		}
	}

	burst := defaultBurst
	if raw := strings.TrimSpace(os.Getenv(burstEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			burst = parsed
		}
	}

	return enabled, rps, burst
}

func newRPCRateLimiter(cfg rpcRateLimitConfig) *rpcRateLimiter {
	if !cfg.Enabled {
		return nil
	}
	return &rpcRateLimiter{
		limiter: ratelimiter.New(cfg.RPS, cfg.Burst, 10*time.Minute),
	}
}

func newFileRateLimiter(cfg fileRateLimitConfig) *rpcRateLimiter {
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
