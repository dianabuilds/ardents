package daemonservice

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"aim-chat/go-backend/internal/domains/contracts"
)

var sizeBuckets = []int{256, 512, 1024, 2048, 4096, 8192}

type outboundMetadataHardening struct {
	enabled      bool
	batchWindow  time.Duration
	jitterMax    time.Duration
	randomMu     sync.Mutex
	randomSource *rand.Rand
}

func newOutboundMetadataHardeningFromEnv() *outboundMetadataHardening {
	p := &outboundMetadataHardening{
		enabled:      true,
		batchWindow:  boundedDurationFromEnv("AIM_BATCH_WINDOW_MS", 80, 0, 200),
		jitterMax:    boundedDurationFromEnv("AIM_JITTER_MAX_MS", 220, 0, 600),
		randomSource: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if raw := strings.TrimSpace(strings.ToLower(os.Getenv("AIM_METADATA_HARDENING"))); raw != "" {
		p.enabled = raw != "0" && raw != "false" && raw != "off"
	}
	return p
}

func boundedDurationFromEnv(key string, def, min, max int) time.Duration {
	value := def
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			value = parsed
		}
	}
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	return time.Duration(value) * time.Millisecond
}

func (p *outboundMetadataHardening) harden(wire contracts.WirePayload) (contracts.WirePayload, time.Duration, error) {
	if p == nil || !p.enabled || p.isLatencyCritical(wire) {
		return wire, 0, nil
	}
	hardened := wire
	hardened.Padding = ""
	target, err := p.targetSize(hardened)
	if err != nil {
		return contracts.WirePayload{}, 0, err
	}
	if target > 0 {
		padding, err := p.buildPaddingForTarget(hardened, target)
		if err != nil {
			return contracts.WirePayload{}, 0, err
		}
		hardened.Padding = padding
	}
	return hardened, p.batchWindow + p.randomJitter(), nil
}

func (p *outboundMetadataHardening) isLatencyCritical(wire contracts.WirePayload) bool {
	switch strings.TrimSpace(strings.ToLower(wire.Kind)) {
	case "receipt", "device_revoke":
		return true
	default:
		return false
	}
}

func (p *outboundMetadataHardening) targetSize(wire contracts.WirePayload) (int, error) {
	raw, err := json.Marshal(wire)
	if err != nil {
		return 0, err
	}
	size := len(raw)
	for _, bucket := range sizeBuckets {
		if size <= bucket {
			return bucket, nil
		}
	}
	return 0, nil
}

func (p *outboundMetadataHardening) buildPaddingForTarget(wire contracts.WirePayload, target int) (string, error) {
	withPad := wire
	low := 0
	high := target
	best := ""
	for low <= high {
		mid := (low + high) / 2
		withPad.Padding = strings.Repeat("0", mid)
		raw, err := json.Marshal(withPad)
		if err != nil {
			return "", err
		}
		size := len(raw)
		if size == target {
			return withPad.Padding, nil
		}
		if size < target {
			best = withPad.Padding
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	withPad.Padding = best
	raw, err := json.Marshal(withPad)
	if err != nil {
		return "", err
	}
	if len(raw) > target {
		return "", errors.New("failed to fit payload into target size bucket")
	}
	return best, nil
}

func (p *outboundMetadataHardening) randomJitter() time.Duration {
	if p == nil || p.jitterMax <= 0 {
		return 0
	}
	p.randomMu.Lock()
	defer p.randomMu.Unlock()
	if p.randomSource == nil {
		return 0
	}
	n := p.randomSource.Int63n(int64(p.jitterMax) + 1)
	return time.Duration(n)
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
