package app

import (
	"errors"
	"testing"

	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func TestDispatchDeviceRevocationFailures(t *testing.T) {
	contacts := []models.Contact{{ID: "c1"}, {ID: "c2"}}
	nextCalls := 0
	failures := DispatchDeviceRevocation("local", contacts, []byte("payload"), func() (string, error) {
		nextCalls++
		if nextCalls == 1 {
			return "", errors.New("id gen failed")
		}
		return "rev-2", nil
	}, func(msg waku.PrivateMessage) error {
		return errors.New("publish failed")
	})

	if len(failures) != 2 {
		t.Fatalf("expected 2 failures, got %d", len(failures))
	}
	if failures[0].Category != "api" {
		t.Fatalf("expected first failure to be api, got %s", failures[0].Category)
	}
	if failures[1].Category != "network" {
		t.Fatalf("expected second failure to be network, got %s", failures[1].Category)
	}
}

func TestBuildDeviceRevocationDeliveryError(t *testing.T) {
	err := BuildDeviceRevocationDeliveryError(2, []RevocationFailure{
		{ContactID: "c1", Err: errors.New("a")},
		{ContactID: "c2", Err: errors.New("b")},
	})
	if err == nil {
		t.Fatal("expected delivery error")
	}
	if !err.IsFullFailure() {
		t.Fatal("expected full failure")
	}
}
