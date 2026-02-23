package nodeagent

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

type DoctorInput struct {
	ListenPort       int
	AdvertiseAddress string
	RPCAddr          string
	RPCToken         string
	MinPeers         int
}

type DoctorCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason,omitempty"`
}

type DoctorReport struct {
	Ready     bool          `json:"ready"`
	Checks    []DoctorCheck `json:"checks"`
	CheckedAt time.Time     `json:"checked_at"`
}

func (s *Service) Doctor(ctx context.Context, input DoctorInput) (DoctorReport, error) {
	if input.MinPeers <= 0 {
		input.MinPeers = 1
	}
	now := s.now()
	report := DoctorReport{
		Ready:     true,
		Checks:    make([]DoctorCheck, 0, 8),
		CheckedAt: now,
	}
	appendCheck := func(name string, pass bool, reason string) {
		report.Checks = append(report.Checks, DoctorCheck{Name: name, Pass: pass, Reason: reason})
		if !pass {
			report.Ready = false
		}
	}

	state, exists, err := s.loadState()
	if err != nil {
		return DoctorReport{}, err
	}
	appendCheck("state_initialized", exists, failReason(!exists, "node-agent is not initialized"))
	appendCheck("state_enrolled", exists && state.Enrollment != nil, failReason(!(exists && state.Enrollment != nil), "node is not enrolled"))

	listenPortValid := input.ListenPort >= 1 && input.ListenPort <= 65535
	appendCheck("listen_port_valid", listenPortValid, failReason(!listenPortValid, "listen port must be in [1..65535]"))

	if listenPortValid {
		if err := checkPortAvailable(input.ListenPort); err != nil {
			appendCheck("listen_port_available", false, err.Error())
		} else {
			appendCheck("listen_port_available", true, "")
		}
	}

	if err := validateAdvertiseAddress(input.AdvertiseAddress); err != nil {
		appendCheck("advertise_address_valid", false, err.Error())
	} else {
		appendCheck("advertise_address_valid", true, "")
	}

	if strings.TrimSpace(input.RPCAddr) != "" {
		peerCount, err := s.probe(ctx, input.RPCAddr, input.RPCToken)
		if err != nil {
			appendCheck("rpc_reachable", false, err.Error())
		} else {
			appendCheck("rpc_reachable", true, "")
			appendCheck("peer_count_min", peerCount >= input.MinPeers, failReason(peerCount < input.MinPeers, fmt.Sprintf("peer_count=%d < min_peers=%d", peerCount, input.MinPeers)))
		}
	}
	return report, nil
}

func failReason(failed bool, reason string) string {
	if !failed {
		return ""
	}
	return reason
}

func checkPortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable: %w", port, err)
	}
	_ = ln.Close()
	return nil
}

func validateAdvertiseAddress(raw string) error {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return nil
	}

	host := addr
	if h, p, err := net.SplitHostPort(addr); err == nil {
		host = strings.TrimSpace(h)
		port, convErr := strconv.Atoi(strings.TrimSpace(p))
		if convErr != nil || port < 1 || port > 65535 {
			return fmt.Errorf("advertise address port is invalid: %q", p)
		}
	}

	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if !hostnamePattern.MatchString(host) {
		return fmt.Errorf("advertise address host is invalid: %q", host)
	}
	return nil
}
