package daemonservice

import (
	"testing"

	"aim-chat/go-backend/internal/waku"
)

func TestCreateIdentityCreatesIsolatedAccountProfileAndAllowsSwitchBack(t *testing.T) {
	cfg := waku.DefaultConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	legacyIdentity, err := svc.GetIdentity()
	if err != nil {
		t.Fatalf("get legacy identity failed: %v", err)
	}

	created, _, err := svc.CreateIdentity("password-1")
	if err != nil {
		t.Fatalf("create identity failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created identity id is empty")
	}

	accounts, err := svc.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts failed: %v", err)
	}
	if len(accounts) < 2 {
		t.Fatalf("expected at least 2 accounts, got %d", len(accounts))
	}
	activeAccountID := ""
	for _, account := range accounts {
		if account.Active {
			activeAccountID = account.ID
			break
		}
	}
	if activeAccountID == "" {
		t.Fatal("active account is not reported")
	}

	if _, err := svc.SwitchAccount(legacyAccountID); err != nil {
		t.Fatalf("switch to legacy failed: %v", err)
	}
	legacyAfterSwitch, err := svc.GetIdentity()
	if err != nil {
		t.Fatalf("get legacy identity after switch failed: %v", err)
	}
	if legacyAfterSwitch.ID != legacyIdentity.ID {
		t.Fatalf("legacy identity mismatch after switch: got=%q want=%q", legacyAfterSwitch.ID, legacyIdentity.ID)
	}

	switched, err := svc.SwitchAccount(activeAccountID)
	if err != nil {
		t.Fatalf("switch back to created account failed: %v", err)
	}
	if switched.ID != created.ID {
		t.Fatalf("created identity mismatch after switch back: got=%q want=%q", switched.ID, created.ID)
	}
}
