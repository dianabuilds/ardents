package manifestruntime

import (
	"errors"
	"math"
	"math/rand"
	"time"
)

type RefreshState string

const (
	StateFresh   RefreshState = "fresh"
	StateStale   RefreshState = "stale"
	StateExpired RefreshState = "expired"
)

type Source string

const (
	SourceManifest Source = "manifest"
	SourceCache    Source = "cache"
	SourceBaked    Source = "baked"
	SourceNone     Source = "none"
)

type ErrorKind string

const (
	ErrorNone           ErrorKind = "none"
	ErrorRecoverable    ErrorKind = "recoverable"
	ErrorNonRecoverable ErrorKind = "non_recoverable"
)

type Config struct {
	RefreshInterval      time.Duration
	StaleRefreshInterval time.Duration
	SlowPollingInterval  time.Duration
	StaleWindow          time.Duration
	BackoffBase          time.Duration
	BackoffMax           time.Duration
	BackoffFactor        float64
	BackoffJitterRatio   float64
}

func (c Config) Normalize() Config {
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = 60 * time.Second
	}
	if c.StaleRefreshInterval <= 0 {
		c.StaleRefreshInterval = 15 * time.Second
	}
	if c.SlowPollingInterval <= 0 {
		c.SlowPollingInterval = 60 * time.Second
	}
	if c.StaleWindow <= 0 {
		c.StaleWindow = 5 * time.Minute
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = 1 * time.Second
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = 30 * time.Second
	}
	if c.BackoffMax < c.BackoffBase {
		c.BackoffMax = c.BackoffBase
	}
	if c.BackoffFactor < 1 {
		c.BackoffFactor = 2.0
	}
	if c.BackoffJitterRatio < 0 {
		c.BackoffJitterRatio = 0
	} else if c.BackoffJitterRatio > 1 {
		c.BackoffJitterRatio = 1
	}
	return c
}

type AttemptOutcome struct {
	ManifestAccepted  bool
	ManifestExpiresAt time.Time
	ErrorKind         ErrorKind
	CacheUsable       bool
	BakedUsable       bool
}

type Decision struct {
	State            RefreshState
	SourceBefore     Source
	SourceAfter      Source
	NextDelay        time.Duration
	FallbackApplied  bool
	RestoredManifest bool
	Reason           string
}

type Controller struct {
	cfg          Config
	rnd          *rand.Rand
	source       Source
	failures     int
	lastExpiryAt time.Time
}

func NewController(cfg Config, rnd *rand.Rand) *Controller {
	normalized := cfg.Normalize()
	if rnd == nil {
		rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Controller{
		cfg:    normalized,
		rnd:    rnd,
		source: SourceManifest,
	}
}

func StateAt(now, expiresAt time.Time, staleWindow time.Duration) RefreshState {
	if expiresAt.IsZero() || !now.Before(expiresAt) {
		return StateExpired
	}
	if now.Before(expiresAt.Add(-staleWindow)) {
		return StateFresh
	}
	return StateStale
}

func SelectFallbackSource(cacheUsable, bakedUsable bool) (Source, error) {
	if cacheUsable {
		return SourceCache, nil
	}
	if bakedUsable {
		return SourceBaked, nil
	}
	return SourceNone, errors.New("bootstrap sources are unavailable")
}

func (c *Controller) Source() Source {
	return c.source
}

func (c *Controller) Decide(now time.Time, outcome AttemptOutcome) Decision {
	before := c.source
	if outcome.ManifestAccepted {
		c.failures = 0
		c.lastExpiryAt = outcome.ManifestExpiresAt
		state := StateAt(now, outcome.ManifestExpiresAt, c.cfg.StaleWindow)
		delay := c.cfg.RefreshInterval
		if state == StateStale {
			delay = c.cfg.StaleRefreshInterval
		}
		c.source = SourceManifest
		return Decision{
			State:            state,
			SourceBefore:     before,
			SourceAfter:      c.source,
			NextDelay:        delay,
			FallbackApplied:  before != SourceManifest,
			RestoredManifest: before != SourceManifest,
			Reason:           "manifest accepted",
		}
	}

	state := StateExpired
	if !c.lastExpiryAt.IsZero() {
		state = StateAt(now, c.lastExpiryAt, c.cfg.StaleWindow)
	}

	fallback, err := SelectFallbackSource(outcome.CacheUsable, outcome.BakedUsable)
	if err == nil {
		c.source = fallback
	} else {
		c.source = SourceNone
	}

	delay := c.cfg.SlowPollingInterval
	if outcome.ErrorKind == ErrorRecoverable {
		c.failures++
		delay = nextBackoff(c.failures, c.cfg.BackoffBase, c.cfg.BackoffMax, c.cfg.BackoffFactor, c.cfg.BackoffJitterRatio, c.rnd)
	} else {
		c.failures = 0
	}

	reason := "manifest rejected"
	if outcome.ErrorKind == ErrorRecoverable {
		reason = "recoverable error"
	} else if outcome.ErrorKind == ErrorNonRecoverable {
		reason = "non-recoverable error"
	}
	if c.source == SourceNone {
		reason = "fallback unavailable"
	}

	return Decision{
		State:            state,
		SourceBefore:     before,
		SourceAfter:      c.source,
		NextDelay:        delay,
		FallbackApplied:  before != c.source && c.source != SourceManifest,
		RestoredManifest: false,
		Reason:           reason,
	}
}

func nextBackoff(attempt int, base, max time.Duration, factor, jitter float64, rnd *rand.Rand) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	value := float64(base) * math.Pow(factor, float64(attempt-1))
	if value > float64(max) {
		value = float64(max)
	}
	if jitter <= 0 || rnd == nil {
		return time.Duration(value)
	}
	delta := value * jitter
	low := value - delta
	high := value + delta
	if low < 0 {
		low = 0
	}
	out := low + rnd.Float64()*(high-low)
	return time.Duration(out)
}
