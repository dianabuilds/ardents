package rpc

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	rpcStreamMaxGlobalEnv    = "AIM_RPC_STREAM_MAX_GLOBAL"
	rpcStreamMaxPerClientEnv = "AIM_RPC_STREAM_MAX_PER_CLIENT"
)

type rpcStreamLimitConfig struct {
	MaxGlobal    int
	MaxPerClient int
}

type rpcStreamLimiter struct {
	maxGlobal    int
	maxPerClient int

	mu       sync.Mutex
	global   int
	byClient map[string]int
}

func loadRPCStreamLimitConfig() rpcStreamLimitConfig {
	cfg := rpcStreamLimitConfig{
		MaxGlobal:    128,
		MaxPerClient: 8,
	}
	if raw := strings.TrimSpace(os.Getenv(rpcStreamMaxGlobalEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.MaxGlobal = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv(rpcStreamMaxPerClientEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.MaxPerClient = parsed
		}
	}
	return cfg
}

func newRPCStreamLimiter(cfg rpcStreamLimitConfig) *rpcStreamLimiter {
	return &rpcStreamLimiter{
		maxGlobal:    cfg.MaxGlobal,
		maxPerClient: cfg.MaxPerClient,
		byClient:     make(map[string]int),
	}
}

func (l *rpcStreamLimiter) acquire(clientKey string) (func(), bool) {
	if l == nil {
		return func() {}, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.global >= l.maxGlobal {
		return nil, false
	}
	if l.byClient[clientKey] >= l.maxPerClient {
		return nil, false
	}
	l.global++
	l.byClient[clientKey]++
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.global > 0 {
			l.global--
		}
		next := l.byClient[clientKey] - 1
		if next <= 0 {
			delete(l.byClient, clientKey)
			return
		}
		l.byClient[clientKey] = next
	}, true
}
