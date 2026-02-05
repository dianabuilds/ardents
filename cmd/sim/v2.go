package main

import (
	"errors"
	mrand "math/rand"
	"os"

	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

func runV2Checks(nPeers int, rng *mrand.Rand) (simReport, error) {
	home, err := os.MkdirTemp("", "ardents-sim-v2-*")
	if err != nil {
		return simReport{}, err
	}
	defer func() {
		_ = os.RemoveAll(home)
	}()
	_ = os.Setenv(appdirs.EnvHome, home)
	report := simReport{
		Checks: map[string]checkResult{},
	}
	if err := checkReseedQuorum(); err != nil {
		report.Checks["reseed_quorum"] = checkResult{OK: false, Error: err.Error()}
	} else {
		report.Checks["reseed_quorum"] = checkResult{OK: true}
	}
	if err := checkNetDBPoisoning(); err != nil {
		report.Checks["netdb_poisoning_reject"] = checkResult{OK: false, Error: err.Error()}
	} else {
		report.Checks["netdb_poisoning_reject"] = checkResult{OK: true}
	}
	if err := checkNetDBWire(rng); err != nil {
		report.Checks["netdb_wire"] = checkResult{OK: false, Error: err.Error()}
	} else {
		report.Checks["netdb_wire"] = checkResult{OK: true}
	}
	if err := checkDirQueryE2E(rng); err != nil {
		report.Checks["dirquery_e2e"] = checkResult{OK: false, Error: err.Error()}
	} else {
		report.Checks["dirquery_e2e"] = checkResult{OK: true}
	}
	latencyP95, err := checkTunnelsAndPadding(nPeers, rng)
	if err != nil {
		report.Checks["tunnel_rotate_padding"] = checkResult{OK: false, Error: err.Error()}
	} else {
		report.Checks["tunnel_rotate_padding"] = checkResult{OK: true}
	}
	report.LatencyP95Ms = latencyP95
	for _, r := range report.Checks {
		if !r.OK {
			return report, errors.New("ERR_SIM_FAILED")
		}
	}
	return report, nil
}
