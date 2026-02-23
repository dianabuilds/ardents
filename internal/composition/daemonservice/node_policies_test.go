package daemonservice

import (
	"testing"

	"aim-chat/go-backend/pkg/models"
)

func TestUpdateNodePoliciesDoesNotCrossAffectSections(t *testing.T) {
	cfg := newMockConfig()
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	initial := svc.GetNodePolicies()
	updated, err := svc.UpdateNodePolicies(models.NodePoliciesPatch{
		Public: &models.NodePublicPolicyPatch{
			StoreEnabled: boolPtr(true),
		},
	})
	if err != nil {
		t.Fatalf("update node policies: %v", err)
	}
	if !updated.Public.StoreEnabled {
		t.Fatalf("expected public store enabled: %+v", updated.Public)
	}
	if updated.Personal != initial.Personal {
		t.Fatalf("personal policy changed unexpectedly: before=%+v after=%+v", initial.Personal, updated.Personal)
	}
}

func TestNodePoliciesPersistAcrossRestart(t *testing.T) {
	cfg := newMockConfig()
	dataDir := t.TempDir()
	svc, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.UpdateNodePolicies(models.NodePoliciesPatch{
		Personal: &models.NodePersonalPolicyPatch{
			QuotaMB: intPtr(2048),
		},
		Public: &models.NodePublicPolicyPatch{
			ServingEnabled: boolPtr(false),
		},
	})
	if err != nil {
		t.Fatalf("update node policies: %v", err)
	}

	reloaded, err := NewServiceForDaemonWithDataDir(cfg, dataDir)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	got := reloaded.GetNodePolicies()
	if got.Personal.QuotaMB != 2048 {
		t.Fatalf("unexpected personal quota after restart: %+v", got.Personal)
	}
	if got.Public.ServingEnabled {
		t.Fatalf("expected public serving disabled after restart: %+v", got.Public)
	}
	if got.ProfileSchemaVersion == 0 {
		t.Fatalf("expected non-zero profile schema version: %+v", got)
	}
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }
