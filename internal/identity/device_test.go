package identity

import (
	"crypto/ed25519"
	"errors"
	"testing"
)

func TestDeviceLifecycleListAddRevoke(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	devices := m.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("expected exactly one primary device, got %d", len(devices))
	}
	if devices[0].ID == "" || devices[0].Name == "" {
		t.Fatal("primary device metadata is empty")
	}

	added, err := m.AddDevice("laptop")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}
	if added.ID == "" {
		t.Fatal("added device id is empty")
	}

	devices = m.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices after add, got %d", len(devices))
	}

	rev, err := m.RevokeDevice(added.ID)
	if err != nil {
		t.Fatalf("revoke device failed: %v", err)
	}
	if rev.DeviceID != added.ID {
		t.Fatal("revocation device id mismatch")
	}

	updated := m.ListDevices()
	found := false
	for _, d := range updated {
		if d.ID == added.ID {
			found = true
			if !d.IsRevoked {
				t.Fatal("device must be marked revoked")
			}
		}
	}
	if !found {
		t.Fatal("revoked device not found in list")
	}
}

func TestRevokedDeviceRejectedForInboundVerification(t *testing.T) {
	alice, err := NewManager()
	if err != nil {
		t.Fatalf("new alice manager failed: %v", err)
	}
	bob, err := NewManager()
	if err != nil {
		t.Fatalf("new bob manager failed: %v", err)
	}

	aliceCard, err := alice.SelfContactCard("alice")
	if err != nil {
		t.Fatalf("alice card failed: %v", err)
	}
	if err := bob.AddContact(aliceCard); err != nil {
		t.Fatalf("bob add alice failed: %v", err)
	}

	secondary, err := alice.AddDevice("old-phone")
	if err != nil {
		t.Fatalf("alice add device failed: %v", err)
	}

	payload := []byte("signed-payload")
	alice.mu.RLock()
	secPriv := append(ed25519.PrivateKey(nil), alice.devices[secondary.ID].priv...)
	alice.mu.RUnlock()
	sig := ed25519.Sign(secPriv, payload)

	if err := bob.VerifyInboundDevice(alice.GetIdentity().ID, secondary, payload, sig); err != nil {
		t.Fatalf("expected secondary device to verify before revoke, got %v", err)
	}

	rev, err := alice.RevokeDevice(secondary.ID)
	if err != nil {
		t.Fatalf("alice revoke failed: %v", err)
	}
	if err := bob.ApplyDeviceRevocation(alice.GetIdentity().ID, rev); err != nil {
		t.Fatalf("bob apply revocation failed: %v", err)
	}

	if err := bob.VerifyInboundDevice(alice.GetIdentity().ID, secondary, payload, sig); !errors.Is(err, ErrDeviceRevoked) {
		t.Fatalf("expected ErrDeviceRevoked, got %v", err)
	}
}

func TestUnverifiedContactRejectedForInboundVerification(t *testing.T) {
	alice, err := NewManager()
	if err != nil {
		t.Fatalf("new alice manager failed: %v", err)
	}
	bob, err := NewManager()
	if err != nil {
		t.Fatalf("new bob manager failed: %v", err)
	}

	if err := bob.AddContactByIdentityID(alice.GetIdentity().ID, "alice"); err != nil {
		t.Fatalf("add contact by id failed: %v", err)
	}

	devices := alice.ListDevices()
	if len(devices) == 0 {
		t.Fatal("alice must have primary device")
	}
	payload := []byte("payload")
	senderDevice, sig, err := alice.ActiveDeviceAuth(payload)
	if err != nil {
		t.Fatalf("active device auth failed: %v", err)
	}
	if senderDevice.ID == "" || len(sig) == 0 {
		t.Fatal("device auth output must not be empty")
	}
	if err := bob.VerifyInboundDevice(alice.GetIdentity().ID, senderDevice, payload, sig); !errors.Is(err, ErrUnverifiedContact) {
		t.Fatalf("expected ErrUnverifiedContact, got %v", err)
	}
}
