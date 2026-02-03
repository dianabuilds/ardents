package main

import (
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"os"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

type v2CheckResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type v2Stats struct {
	Checks         map[string]v2CheckResult `json:"checks"`
	LatencyP95Ms   int64                    `json:"latency_p95_ms"`
	DurationMillis int64                    `json:"duration_ms"`
}

func runV2(nPeers int, rng *mrand.Rand) error {
	home, err := os.MkdirTemp("", "ardents-sim-v2-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(home)
	}()
	_ = os.Setenv(appdirs.EnvHome, home)
	start := time.Now()
	stats := v2Stats{
		Checks: map[string]v2CheckResult{},
	}
	if err := checkReseedQuorum(); err != nil {
		stats.Checks["reseed_quorum"] = v2CheckResult{OK: false, Error: err.Error()}
	} else {
		stats.Checks["reseed_quorum"] = v2CheckResult{OK: true}
	}
	if err := checkNetDBPoisoning(); err != nil {
		stats.Checks["netdb_poisoning_reject"] = v2CheckResult{OK: false, Error: err.Error()}
	} else {
		stats.Checks["netdb_poisoning_reject"] = v2CheckResult{OK: true}
	}
	if err := checkNetDBWire(rng); err != nil {
		stats.Checks["netdb_wire"] = v2CheckResult{OK: false, Error: err.Error()}
	} else {
		stats.Checks["netdb_wire"] = v2CheckResult{OK: true}
	}
	if err := checkDirQueryE2E(rng); err != nil {
		stats.Checks["dirquery_e2e"] = v2CheckResult{OK: false, Error: err.Error()}
	} else {
		stats.Checks["dirquery_e2e"] = v2CheckResult{OK: true}
	}
	latencyP95, err := checkTunnelsAndPadding(nPeers, rng)
	if err != nil {
		stats.Checks["tunnel_rotate_padding"] = v2CheckResult{OK: false, Error: err.Error()}
	} else {
		stats.Checks["tunnel_rotate_padding"] = v2CheckResult{OK: true}
	}
	stats.LatencyP95Ms = latencyP95
	stats.DurationMillis = time.Since(start).Milliseconds()

	for _, r := range stats.Checks {
		if !r.OK {
			printV2Stats(stats)
			return errors.New("ERR_SIM_FAILED")
		}
	}
	printV2Stats(stats)
	return nil
}

func printV2Stats(st v2Stats) {
	b, _ := json.MarshalIndent(st, "", "  ")
	fmt.Println(string(b))
}
