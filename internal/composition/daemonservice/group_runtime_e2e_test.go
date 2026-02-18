package daemonservice

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	groupdomain "aim-chat/go-backend/internal/domains/group"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func TestRuntimeE2E_GroupChatThreeProfiles(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	baseDir := t.TempDir()
	makeService := func(name string) *Service {
		svc, err := NewServiceForDaemonWithDataDir(cfg, filepath.Join(baseDir, name))
		if err != nil {
			t.Fatalf("new service %s: %v", name, err)
		}
		return svc
	}

	alice := makeService("alice")
	bob := makeService("bob")
	charlie := makeService("charlie")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	defer func() { _ = alice.StopNetworking(stopCtx) }()
	defer func() { _ = bob.StopNetworking(stopCtx) }()
	defer func() { _ = charlie.StopNetworking(stopCtx) }()

	aliceIdentity, err := alice.GetIdentity()
	if err != nil {
		t.Fatalf("alice identity: %v", err)
	}
	bobIdentity, err := bob.GetIdentity()
	if err != nil {
		t.Fatalf("bob identity: %v", err)
	}
	charlieIdentity, err := charlie.GetIdentity()
	if err != nil {
		t.Fatalf("charlie identity: %v", err)
	}

	aliceCard, err := alice.SelfContactCard("Alice")
	if err != nil {
		t.Fatalf("alice self card: %v", err)
	}
	bobCard, err := bob.SelfContactCard("Bob")
	if err != nil {
		t.Fatalf("bob self card: %v", err)
	}
	charlieCard, err := charlie.SelfContactCard("Charlie")
	if err != nil {
		t.Fatalf("charlie self card: %v", err)
	}

	mustAddContactCard(t, alice, bobCard)
	mustAddContactCard(t, alice, charlieCard)
	mustAddContactCard(t, bob, aliceCard)
	mustAddContactCard(t, charlie, aliceCard)

	mustInitPairSession(t, alice, aliceIdentity.ID, aliceCard.PublicKey, bob, bobIdentity.ID, bobCard.PublicKey)
	mustInitPairSession(t, alice, aliceIdentity.ID, aliceCard.PublicKey, charlie, charlieIdentity.ID, charlieCard.PublicKey)

	groupID := "group_runtime_e2e_three_profiles"
	seed := seededActiveGroupState(groupID, "Runtime E2E Group", aliceIdentity.ID, []string{
		aliceIdentity.ID,
		bobIdentity.ID,
		charlieIdentity.ID,
	})
	applySeedGroupState(groupID, seed, alice, bob, charlie)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustStartNetworkingNamed(t, ctx,
		namedRuntimeService{name: "alice", svc: alice},
		namedRuntimeService{name: "bob", svc: bob},
		namedRuntimeService{name: "charlie", svc: charlie},
	)

	messageText := "runtime-group-e2e-" + time.Now().UTC().Format("20060102150405.000000000")
	fanout, err := alice.SendGroupMessage(groupID, messageText)
	if err != nil {
		t.Fatalf("alice send group message: %v", err)
	}
	if fanout.Attempted != 2 {
		t.Fatalf("unexpected fanout attempted: got=%d want=2", fanout.Attempted)
	}
	if fanout.Failed != 0 {
		t.Fatalf("unexpected fanout failures: %+v", fanout)
	}

	waitForGroupMessage(t, bob, groupID, messageText)
	waitForGroupMessage(t, charlie, groupID, messageText)
}

func TestRuntimeE2E_ChannelPublishPermissions(t *testing.T) {
	t.Parallel()

	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock

	baseDir := t.TempDir()
	makeService := func(name string) *Service {
		svc, err := NewServiceForDaemonWithDataDir(cfg, filepath.Join(baseDir, name))
		if err != nil {
			t.Fatalf("new service %s: %v", name, err)
		}
		return svc
	}

	alice := makeService("alice")
	bob := makeService("bob")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	defer func() { _ = alice.StopNetworking(stopCtx) }()
	defer func() { _ = bob.StopNetworking(stopCtx) }()

	aliceIdentity, err := alice.GetIdentity()
	if err != nil {
		t.Fatalf("alice identity: %v", err)
	}
	bobIdentity, err := bob.GetIdentity()
	if err != nil {
		t.Fatalf("bob identity: %v", err)
	}

	aliceCard, err := alice.SelfContactCard("Alice")
	if err != nil {
		t.Fatalf("alice self card: %v", err)
	}
	bobCard, err := bob.SelfContactCard("Bob")
	if err != nil {
		t.Fatalf("bob self card: %v", err)
	}

	mustAddContactCard(t, alice, bobCard)
	mustAddContactCard(t, bob, aliceCard)
	mustInitPairSession(t, alice, aliceIdentity.ID, aliceCard.PublicKey, bob, bobIdentity.ID, bobCard.PublicKey)

	groupID := "group_runtime_e2e_channel_permissions"
	seed := seededActiveGroupState(groupID, "[channel:public] Runtime E2E Channel", aliceIdentity.ID, []string{
		aliceIdentity.ID,
		bobIdentity.ID,
	})
	applySeedGroupState(groupID, seed, alice, bob)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mustStartNetworkingNamed(t, ctx,
		namedRuntimeService{name: "alice", svc: alice},
		namedRuntimeService{name: "bob", svc: bob},
	)

	ownerMsg := "runtime-channel-owner-" + time.Now().UTC().Format("20060102150405.000000000")
	if _, err := alice.SendGroupMessage(groupID, ownerMsg); err != nil {
		t.Fatalf("alice send channel message: %v", err)
	}
	waitForGroupMessage(t, bob, groupID, ownerMsg)

	if _, err := bob.SendGroupMessage(groupID, "runtime-channel-user-denied"); err == nil {
		t.Fatalf("expected permission error for user publish")
	} else if !strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		t.Fatalf("expected permission denied error, got: %v", err)
	}
}

func mustAddContactCard(t *testing.T, svc *Service, card models.ContactCard) {
	t.Helper()
	if err := svc.AddContactCard(card); err != nil {
		t.Fatalf("add contact card %s: %v", card.IdentityID, err)
	}
}

func mustInitSession(t *testing.T, svc *Service, contactID string, peerPublicKey []byte) {
	t.Helper()
	if _, err := svc.InitSession(contactID, peerPublicKey); err != nil {
		t.Fatalf("init session %s: %v", contactID, err)
	}
}

func mustInitPairSession(
	t *testing.T,
	left *Service,
	leftID string,
	leftPublicKey []byte,
	right *Service,
	rightID string,
	rightPublicKey []byte,
) {
	t.Helper()
	sessionKey := rightPublicKey
	if leftID <= rightID {
		sessionKey = leftPublicKey
	}
	mustInitSession(t, left, rightID, sessionKey)
	mustInitSession(t, right, leftID, sessionKey)
}

func seededActiveGroupState(groupID, title, ownerID string, memberIDs []string) groupdomain.GroupState {
	now := time.Now().UTC()
	state := groupdomain.NewGroupState(groupdomain.Group{
		ID:        groupID,
		Title:     title,
		CreatedBy: ownerID,
		CreatedAt: now,
		UpdatedAt: now,
	})
	state.Version = 3
	state.LastKeyVersion = 1
	state.AppliedEventIDs = map[string]struct{}{
		"seed-1": {},
		"seed-2": {},
		"seed-3": {},
	}
	for _, memberID := range memberIDs {
		role := groupdomain.GroupMemberRoleUser
		if memberID == ownerID {
			role = groupdomain.GroupMemberRoleOwner
		}
		state.Members[memberID] = groupdomain.GroupMember{
			GroupID:     groupID,
			MemberID:    memberID,
			Role:        role,
			Status:      groupdomain.GroupMemberStatusActive,
			InvitedAt:   now,
			ActivatedAt: now,
			UpdatedAt:   now,
		}
	}
	return state
}

func waitForGroupMessage(t *testing.T, svc *Service, groupID, expectedText string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		messages, err := svc.ListGroupMessages(groupID, 100, 0)
		if err != nil {
			t.Fatalf("list group messages %s: %v", groupID, err)
		}
		for _, msg := range messages {
			if string(msg.Content) == expectedText {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("group message %q was not delivered for group %s", expectedText, groupID)
}

type namedRuntimeService struct {
	name string
	svc  *Service
}

func applySeedGroupState(groupID string, seed groupdomain.GroupState, services ...*Service) {
	for _, svc := range services {
		svc.groupRuntime.SetSnapshot(
			map[string]groupdomain.GroupState{groupID: seed},
			map[string][]groupdomain.GroupEvent{groupID: {}},
		)
	}
}

func mustStartNetworkingNamed(t *testing.T, ctx context.Context, services ...namedRuntimeService) {
	t.Helper()
	for _, runtimeSvc := range services {
		if err := runtimeSvc.svc.StartNetworking(ctx); err != nil {
			t.Fatalf("%s start networking: %v", runtimeSvc.name, err)
		}
	}
}
