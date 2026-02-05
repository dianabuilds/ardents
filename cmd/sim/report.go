package main

import (
	"encoding/json"
	"fmt"
)

type checkResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type simReport struct {
	Checks       map[string]checkResult `json:"checks,omitempty"`
	Traffic      map[string]any         `json:"traffic,omitempty"`
	LatencyP95Ms int64                  `json:"latency_p95_ms,omitempty"`
	DurationMs   int64                  `json:"duration_ms"`
}

func printReport(st simReport) {
	b, _ := json.MarshalIndent(st, "", "  ")
	fmt.Println(string(b))
}
