package nodeagent

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDoctorDetectsUnavailablePort(t *testing.T) {
	svc := New(t.TempDir())
	if _, _, err := svc.Init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	state, exists, err := svc.loadState()
	if err != nil || !exists {
		t.Fatalf("load state failed: exists=%v err=%v", exists, err)
	}
	state.Enrollment = &EnrollmentState{
		TokenID:          "tok-1",
		Issuer:           "ardents-control-plane",
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		KeyID:            "issuer-k1",
		ExpiresAt:        time.Now().UTC().Add(10 * time.Minute),
		EnrolledAt:       time.Now().UTC(),
	}
	if err := svc.saveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen temp port: %v", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			t.Logf("close temp listener: %v", closeErr)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	report, err := svc.Doctor(context.Background(), DoctorInput{ListenPort: port, AdvertiseAddress: "127.0.0.1"})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected readiness fail for unavailable port, report=%+v", report)
	}
	assertCheck(t, report, "listen_port_available", false)
}

func TestDoctorDetectsInvalidAdvertiseOrListen(t *testing.T) {
	svc := New(t.TempDir())
	report, err := svc.Doctor(context.Background(), DoctorInput{
		ListenPort:       -1,
		AdvertiseAddress: "%bad-host%",
	})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected readiness fail, report=%+v", report)
	}
	assertCheck(t, report, "listen_port_valid", false)
	assertCheck(t, report, "advertise_address_valid", false)
}

func TestDoctorPassesReadyNode(t *testing.T) {
	svc := New(t.TempDir())
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	if _, _, err := svc.Init(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	state, exists, err := svc.loadState()
	if err != nil || !exists {
		t.Fatalf("load state failed: exists=%v err=%v", exists, err)
	}
	state.Enrollment = &EnrollmentState{
		TokenID:          "tok-1",
		Issuer:           "ardents-control-plane",
		Scope:            "node.enroll",
		SubjectNodeGroup: "default",
		KeyID:            "issuer-k1",
		ExpiresAt:        now.Add(10 * time.Minute),
		EnrolledAt:       now,
	}
	if err := svc.saveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	svc.probe = func(context.Context, string, string) (int, error) { return 3, nil }

	report, err := svc.Doctor(context.Background(), DoctorInput{
		ListenPort:       freePort(t),
		AdvertiseAddress: "127.0.0.1",
		RPCAddr:          "127.0.0.1:8787",
		MinPeers:         1,
	})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected readiness pass, report=%+v", report)
	}
	assertCheck(t, report, "peer_count_min", true)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("alloc free port: %v", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			t.Logf("close temp listener: %v", closeErr)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func assertCheck(t *testing.T, report DoctorReport, name string, pass bool) {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			if c.Pass != pass {
				t.Fatalf("check %s expected pass=%v got=%v report=%+v", name, pass, c.Pass, report)
			}
			return
		}
	}
	t.Fatalf("check %s not found in report=%+v", name, report)
}
