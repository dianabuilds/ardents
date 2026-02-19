package daemonservice

import (
	"os"
	"strconv"
	"strings"
)

func envString(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envCSV(key string) []string {
	raw := envString(key)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func envBoolWithFallback(key string, fallback bool) bool {
	raw := strings.ToLower(envString(key))
	switch raw {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envIntWithFallback(key string, fallback int) int {
	raw := envString(key)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoundedIntWithFallback(key string, fallback, min, max int) int {
	value := envIntWithFallback(key, fallback)
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
