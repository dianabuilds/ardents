package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aim-chat/go-backend/internal/app"
	"aim-chat/go-backend/internal/app/contracts"
	daemoncomposition "aim-chat/go-backend/internal/composition/daemon"
	"aim-chat/go-backend/internal/crypto"
	"aim-chat/go-backend/internal/identity"
	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/internal/waku"
	"aim-chat/go-backend/pkg/models"
)

func generateSignedContactCard(t *testing.T, displayName string) (models.ContactCard, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	contactID, err := identity.BuildIdentityID(pub)
	if err != nil {
		t.Fatalf("build id failed: %v", err)
	}
	card, err := identity.SignContactCard(contactID, displayName, pub, priv)
	if err != nil {
		t.Fatalf("sign card failed: %v", err)
	}
	return card, contactID
}

func newServicePairWithMutualContacts(t *testing.T) (alice, bob *Service, aliceCard, bobCard models.ContactCard) {
	t.Helper()
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err = NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	bobCard, err = bob.SelfContactCard("bob")
	if err != nil {
		t.Fatalf("bob card failed: %v", err)
	}
	if err := alice.AddContactCard(bobCard); err != nil {
		t.Fatalf("alice add bob contact failed: %v", err)
	}
	aliceCard, err = alice.SelfContactCard("alice")
	if err != nil {
		t.Fatalf("alice card failed: %v", err)
	}
	if err := bob.AddContactCard(aliceCard); err != nil {
		t.Fatalf("bob add alice contact failed: %v", err)
	}
	return alice, bob, aliceCard, bobCard
}

func startServicePairNetworking(t *testing.T, alice, bob *Service) {
	t.Helper()
	if err := alice.StartNetworking(context.Background()); err != nil {
		t.Fatalf("alice start networking failed: %v", err)
	}
	if err := bob.StartNetworking(context.Background()); err != nil {
		t.Fatalf("bob start networking failed: %v", err)
	}
	t.Cleanup(func() { _ = alice.StopNetworking(context.Background()) })
	t.Cleanup(func() { _ = bob.StopNetworking(context.Background()) })
}

func TestServiceGetIdentity(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	id, err := svc.GetIdentity()
	if err != nil {
		t.Fatalf("get identity failed: %v", err)
	}
	if id.ID == "" {
		t.Fatal("identity id must not be empty")
	}
}

func TestServiceDaemonIdentityPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()

	svc1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	id1, err := svc1.GetIdentity()
	if err != nil {
		t.Fatalf("get identity #1 failed: %v", err)
	}
	if id1.ID == "" {
		t.Fatal("identity #1 id must not be empty")
	}

	svc2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	id2, err := svc2.GetIdentity()
	if err != nil {
		t.Fatalf("get identity #2 failed: %v", err)
	}
	if id2.ID == "" {
		t.Fatal("identity #2 id must not be empty")
	}
	if id1.ID != id2.ID {
		t.Fatalf("identity must persist across restart: %s != %s", id1.ID, id2.ID)
	}
}

func TestServiceDaemonPrivacySettingsPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()

	svc1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	if err := svc1.privacyState.Persist(app.PrivacySettings{MessagePrivacyMode: app.MessagePrivacyEveryone}); err != nil {
		t.Fatalf("persist privacy settings failed: %v", err)
	}

	svc2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	if svc2.privacySettings.MessagePrivacyMode != app.MessagePrivacyEveryone {
		t.Fatalf("privacy settings must persist across restart: got=%q want=%q", svc2.privacySettings.MessagePrivacyMode, app.MessagePrivacyEveryone)
	}
}

func TestServiceDaemonPrivacyBootstrapIOErrorDoesNotCrash(t *testing.T) {
	cfg := waku.DefaultConfig()
	dataDir := t.TempDir()
	_, secret, bundle, err := daemoncomposition.ResolveStorage(dataDir)
	if err != nil {
		t.Fatalf("resolve storage failed: %v", err)
	}
	bundle.PrivacyPath = t.TempDir() // directory path forces io error inside Bootstrap

	svc, err := newServiceForDaemonWithBundle(cfg, bundle, secret)
	if err != nil {
		t.Fatalf("service must not fail on privacy io bootstrap error: %v", err)
	}
	if svc.privacySettings.MessagePrivacyMode != app.DefaultMessagePrivacyMode {
		t.Fatalf("expected default privacy mode on bootstrap error: got=%q want=%q", svc.privacySettings.MessagePrivacyMode, app.DefaultMessagePrivacyMode)
	}
}

func TestServicePrivacySettingsGetAndUpdate(t *testing.T) {
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("new daemon service failed: %v", err)
	}

	initial, err := svc.GetPrivacySettings()
	if err != nil {
		t.Fatalf("get privacy settings failed: %v", err)
	}
	if initial.MessagePrivacyMode != app.DefaultMessagePrivacyMode {
		t.Fatalf("unexpected initial privacy mode: got=%q want=%q", initial.MessagePrivacyMode, app.DefaultMessagePrivacyMode)
	}

	updated, err := svc.UpdatePrivacySettings(string(app.MessagePrivacyEveryone))
	if err != nil {
		t.Fatalf("update privacy settings failed: %v", err)
	}
	if updated.MessagePrivacyMode != app.MessagePrivacyEveryone {
		t.Fatalf("unexpected updated mode: got=%q want=%q", updated.MessagePrivacyMode, app.MessagePrivacyEveryone)
	}

	got, err := svc.GetPrivacySettings()
	if err != nil {
		t.Fatalf("get privacy settings after update failed: %v", err)
	}
	if got.MessagePrivacyMode != app.MessagePrivacyEveryone {
		t.Fatalf("unexpected persisted mode in memory: got=%q want=%q", got.MessagePrivacyMode, app.MessagePrivacyEveryone)
	}
}

func TestServicePrivacySettingsUpdateRejectsInvalidMode(t *testing.T) {
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("new daemon service failed: %v", err)
	}

	_, err = svc.UpdatePrivacySettings("invalid")
	if err == nil {
		t.Fatal("expected invalid privacy mode error")
	}
	if !errors.Is(err, app.ErrInvalidMessagePrivacyMode) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceDaemonPrivacySettingsUpdatedValuePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()

	svc1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	if _, err := svc1.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy settings failed: %v", err)
	}

	svc2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	got, err := svc2.GetPrivacySettings()
	if err != nil {
		t.Fatalf("get privacy settings failed: %v", err)
	}
	if got.MessagePrivacyMode != app.MessagePrivacyRequests {
		t.Fatalf("privacy settings must persist across restart: got=%q want=%q", got.MessagePrivacyMode, app.MessagePrivacyRequests)
	}
}

func TestServiceDaemonBlocklistPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()
	blockedID := "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"

	svc1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	if err := svc1.blocklist.Add(blockedID); err != nil {
		t.Fatalf("blocklist add failed: %v", err)
	}
	if err := svc1.blocklistState.Persist(svc1.blocklist); err != nil {
		t.Fatalf("persist blocklist failed: %v", err)
	}

	svc2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	if !svc2.blocklist.Contains(blockedID) {
		t.Fatal("blocklist must persist across restart")
	}
}

func TestServiceDaemonMessageRequestInboxPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()

	alice1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	if _, err := alice1.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	aliceID, _ := alice1.GetIdentity()
	bobID, _ := bob.GetIdentity()

	wireData, err := signedPlainWire(bob, "msg-requests-persist-1", "persist me", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice1.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-persist-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	requestsBefore, err := alice1.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests before restart failed: %v", err)
	}
	if len(requestsBefore) != 1 {
		t.Fatalf("expected one request before restart, got %d", len(requestsBefore))
	}

	alice2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	requestsAfter, err := alice2.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests after restart failed: %v", err)
	}
	if len(requestsAfter) != 1 {
		t.Fatalf("expected one request after restart, got %d", len(requestsAfter))
	}
	if requestsAfter[0].SenderID != bobID.ID {
		t.Fatalf("unexpected sender after restart: got=%s want=%s", requestsAfter[0].SenderID, bobID.ID)
	}
	thread, err := alice2.GetMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("get request thread failed: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("expected one message in restored thread, got %d", len(thread.Messages))
	}
	if string(thread.Messages[0].Content) != "persist me" {
		t.Fatalf("unexpected restored message content: got=%q", string(thread.Messages[0].Content))
	}
}

func TestServiceDaemonMessageRequestActionsPersistAcrossRestart(t *testing.T) {
	tcs := []struct {
		name string
		run  func(*Service, string) error
	}{
		{
			name: "accept",
			run: func(svc *Service, senderID string) error {
				_, err := svc.AcceptMessageRequest(senderID)
				return err
			},
		},
		{
			name: "decline",
			run: func(svc *Service, senderID string) error {
				_, err := svc.DeclineMessageRequest(senderID)
				return err
			},
		},
		{
			name: "block",
			run: func(svc *Service, senderID string) error {
				_, err := svc.BlockSender(senderID)
				return err
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := waku.DefaultConfig()

			alice1, err := NewServiceForDaemonWithDataDir(cfg, dir)
			if err != nil {
				t.Fatalf("new daemon service #1 failed: %v", err)
			}
			if _, err := alice1.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
				t.Fatalf("update privacy mode failed: %v", err)
			}
			bob, err := NewService()
			if err != nil {
				t.Fatalf("new bob service failed: %v", err)
			}
			aliceID, _ := alice1.GetIdentity()
			bobID, _ := bob.GetIdentity()

			wireData, err := signedPlainWire(bob, "msg-requests-action-"+tc.name, "pending", bobID.ID, aliceID.ID)
			if err != nil {
				t.Fatalf("build wire failed: %v", err)
			}
			alice1.handleIncomingPrivateMessage(waku.PrivateMessage{
				ID:        "msg-requests-action-" + tc.name,
				SenderID:  bobID.ID,
				Recipient: aliceID.ID,
				Payload:   wireData,
			})

			alice2, err := NewServiceForDaemonWithDataDir(cfg, dir)
			if err != nil {
				t.Fatalf("new daemon service #2 failed: %v", err)
			}
			if err := tc.run(alice2, bobID.ID); err != nil {
				t.Fatalf("apply %s failed: %v", tc.name, err)
			}

			alice3, err := NewServiceForDaemonWithDataDir(cfg, dir)
			if err != nil {
				t.Fatalf("new daemon service #3 failed: %v", err)
			}
			requestsAfter, err := alice3.ListMessageRequests()
			if err != nil {
				t.Fatalf("list requests after %s and restart failed: %v", tc.name, err)
			}
			if len(requestsAfter) != 0 {
				t.Fatalf("%s must persist request inbox cleanup, got %d pending requests", tc.name, len(requestsAfter))
			}

			switch tc.name {
			case "block":
				blocked, err := alice3.GetBlocklist()
				if err != nil {
					t.Fatalf("get blocklist failed: %v", err)
				}
				found := false
				for _, id := range blocked {
					if id == bobID.ID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("block must persist sender in blocklist: %s", bobID.ID)
				}
			}
		})
	}
}

func TestServiceBlocklistAddRemoveAndListDeterministic(t *testing.T) {
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("new daemon service failed: %v", err)
	}
	first := "aim1UUMgCUXE93BxtwVDUivN2q3eYPKwaPkqjnNp9QVV9pF"
	second := "aim1fve68neM6h8bGBw6QWB5h58SdzD6XraY7fJfxfdnRSe"

	blocked, err := svc.AddToBlocklist(second)
	if err != nil {
		t.Fatalf("add second to blocklist failed: %v", err)
	}
	if len(blocked) != 1 || blocked[0] != second {
		t.Fatalf("unexpected blocked list after first add: %#v", blocked)
	}

	blocked, err = svc.AddToBlocklist(first)
	if err != nil {
		t.Fatalf("add first to blocklist failed: %v", err)
	}
	if len(blocked) != 2 || blocked[0] != first || blocked[1] != second {
		t.Fatalf("blocked list must be deterministic and sorted: %#v", blocked)
	}

	got, err := svc.GetBlocklist()
	if err != nil {
		t.Fatalf("get blocklist failed: %v", err)
	}
	if len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("unexpected blocklist: %#v", got)
	}

	blocked, err = svc.RemoveFromBlocklist(first)
	if err != nil {
		t.Fatalf("remove first from blocklist failed: %v", err)
	}
	if len(blocked) != 1 || blocked[0] != second {
		t.Fatalf("unexpected blocklist after remove: %#v", blocked)
	}
}

func TestServiceBlocklistRejectsInvalidIdentityID(t *testing.T) {
	svc, err := NewServiceForDaemonWithDataDir(waku.DefaultConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("new daemon service failed: %v", err)
	}

	_, err = svc.AddToBlocklist("invalid")
	if err == nil {
		t.Fatal("expected invalid identity id error")
	}
	if !errors.Is(err, app.ErrInvalidIdentityID) {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.RemoveFromBlocklist("invalid")
	if err == nil {
		t.Fatal("expected invalid identity id error on remove")
	}
	if !errors.Is(err, app.ErrInvalidIdentityID) {
		t.Fatalf("unexpected remove error: %v", err)
	}
}

func TestServiceDaemonBlocklistBootstrapIOErrorDoesNotCrash(t *testing.T) {
	cfg := waku.DefaultConfig()
	dataDir := t.TempDir()
	_, secret, bundle, err := daemoncomposition.ResolveStorage(dataDir)
	if err != nil {
		t.Fatalf("resolve storage failed: %v", err)
	}
	bundle.BlocklistPath = t.TempDir() // directory path forces io error inside Bootstrap

	svc, err := newServiceForDaemonWithBundle(cfg, bundle, secret)
	if err != nil {
		t.Fatalf("service must not fail on blocklist io bootstrap error: %v", err)
	}
	if len(svc.blocklist.List()) != 0 {
		t.Fatalf("expected empty blocklist on bootstrap error, got %d", len(svc.blocklist.List()))
	}
}

func TestServiceDaemonTrustStateDeterministicAfterRestart(t *testing.T) {
	dir := t.TempDir()
	cfg := waku.DefaultConfig()

	svc1, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #1 failed: %v", err)
	}
	id1, err := svc1.GetIdentity()
	if err != nil {
		t.Fatalf("get identity #1 failed: %v", err)
	}
	if id1.ID == "" {
		t.Fatal("identity #1 id must not be empty")
	}

	if err := svc1.AddContact("aim1truststatecontact01", "contact-1"); err != nil {
		t.Fatalf("add contact failed: %v", err)
	}
	added, err := svc1.AddDevice("tablet")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}
	if _, err := svc1.RevokeDevice(added.ID); err != nil {
		if _, _, _, ok := DeviceRevocationDeliveryStats(err); !ok {
			t.Fatalf("revoke device failed: %v", err)
		}
	}

	contactsBefore, err := svc1.GetContacts()
	if err != nil {
		t.Fatalf("get contacts before restart failed: %v", err)
	}
	if len(contactsBefore) != 1 {
		t.Fatalf("expected one contact before restart, got %d", len(contactsBefore))
	}
	devicesBefore, err := svc1.ListDevices()
	if err != nil {
		t.Fatalf("list devices before restart failed: %v", err)
	}
	if len(devicesBefore) < 2 {
		t.Fatalf("expected at least two devices before restart, got %d", len(devicesBefore))
	}

	svc2, err := NewServiceForDaemonWithDataDir(cfg, dir)
	if err != nil {
		t.Fatalf("new daemon service #2 failed: %v", err)
	}
	id2, err := svc2.GetIdentity()
	if err != nil {
		t.Fatalf("get identity #2 failed: %v", err)
	}
	if id1.ID != id2.ID {
		t.Fatalf("identity must persist across restart: %s != %s", id1.ID, id2.ID)
	}

	contactsAfter, err := svc2.GetContacts()
	if err != nil {
		t.Fatalf("get contacts after restart failed: %v", err)
	}
	if len(contactsAfter) != 0 {
		t.Fatalf("expected no contacts after restart, got %d", len(contactsAfter))
	}
	devicesAfter, err := svc2.ListDevices()
	if err != nil {
		t.Fatalf("list devices after restart failed: %v", err)
	}
	if len(devicesAfter) != 1 {
		t.Fatalf("expected exactly one primary device after restart, got %d", len(devicesAfter))
	}
	if devicesAfter[0].IsRevoked {
		t.Fatal("primary device must not be revoked after restart")
	}
}

func TestServiceAddAndVerifyContactCard(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	card, _ := generateSignedContactCard(t, "bob")

	ok, err := svc.VerifyContactCard(card)
	if err != nil {
		t.Fatalf("verify card failed: %v", err)
	}
	if !ok {
		t.Fatal("verify should return true for valid card")
	}

	if err := svc.AddContactCard(card); err != nil {
		t.Fatalf("add contact card failed: %v", err)
	}
	contacts, err := svc.GetContacts()
	if err != nil {
		t.Fatalf("get contacts failed: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
}

func TestServiceNetworkLifecycleStatus(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	initial := svc.GetNetworkStatus()
	if initial.Status != "disconnected" {
		t.Fatalf("expected disconnected before start, got %s", initial.Status)
	}

	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	started := svc.GetNetworkStatus()
	if started.Status != "connected" {
		t.Fatalf("expected connected after start, got %s", started.Status)
	}
	if started.PeerCount <= 0 {
		t.Fatalf("expected peer count > 0 after start, got %d", started.PeerCount)
	}

	if err := svc.StopNetworking(context.Background()); err != nil {
		t.Fatalf("stop networking failed: %v", err)
	}
	stopped := svc.GetNetworkStatus()
	if stopped.Status != "disconnected" {
		t.Fatalf("expected disconnected after stop, got %s", stopped.Status)
	}
}

func TestServiceInitSession(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	card, contactID := generateSignedContactCard(t, "bob")
	if err := svc.AddContactCard(card); err != nil {
		t.Fatalf("add contact card failed: %v", err)
	}

	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(i + 1)
	}
	session, err := svc.InitSession(contactID, peerKey)
	if err != nil {
		t.Fatalf("init session failed: %v", err)
	}
	if session.SessionID == "" {
		t.Fatal("session id must not be empty")
	}

	if _, err := svc.InitSession(contactID, []byte{1, 2, 3}); err == nil {
		t.Fatal("expected invalid peer key to fail")
	}
}

func TestServiceSendReceiveMessageAcrossTwoNodes(t *testing.T) {
	alice, bob, _, bobCard := newServicePairWithMutualContacts(t)
	startServicePairNetworking(t, alice, bob)

	if _, err := alice.SendMessage(bobCard.IdentityID, "hello bob"); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	aliceIdentity, err := alice.GetIdentity()
	if err != nil {
		t.Fatalf("alice get identity failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		msgs, err := bob.GetMessages(aliceIdentity.ID, 10, 0)
		if err != nil {
			t.Fatalf("bob get messages failed: %v", err)
		}
		if len(msgs) > 0 {
			if string(msgs[0].Content) != "hello bob" {
				t.Fatalf("unexpected message content: %s", string(msgs[0].Content))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for bob to receive message")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceSendReceiveMessageE2EE(t *testing.T) {
	alice, bob, _, _ := newServicePairWithMutualContacts(t)
	bobIdentity, err := bob.GetIdentity()
	if err != nil {
		t.Fatalf("bob get identity failed: %v", err)
	}

	aliceIdentity, err := alice.GetIdentity()
	if err != nil {
		t.Fatalf("alice get identity failed: %v", err)
	}

	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(i + 50)
	}
	if _, err := alice.InitSession(bobIdentity.ID, peerKey); err != nil {
		t.Fatalf("alice init session failed: %v", err)
	}
	if _, err := bob.InitSession(aliceIdentity.ID, peerKey); err != nil {
		t.Fatalf("bob init session failed: %v", err)
	}

	startServicePairNetworking(t, alice, bob)

	if _, err := alice.SendMessage(bobIdentity.ID, "secret over ratchet"); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		msgs, err := bob.GetMessages(aliceIdentity.ID, 10, 0)
		if err != nil {
			t.Fatalf("bob get messages failed: %v", err)
		}
		if len(msgs) > 0 {
			if msgs[0].ContentType != "e2ee" {
				t.Fatalf("expected e2ee content type, got %s", msgs[0].ContentType)
			}
			if string(msgs[0].Content) != "secret over ratchet" {
				t.Fatalf("unexpected decrypted content: %s", string(msgs[0].Content))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for e2ee message")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceSendMessageCreatesPendingWhenDisconnected(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	bobCard, err := bob.SelfContactCard("bob")
	if err != nil {
		t.Fatalf("bob card failed: %v", err)
	}
	if err := alice.AddContactCard(bobCard); err != nil {
		t.Fatalf("alice add contact failed: %v", err)
	}

	msgID, err := alice.SendMessage(bobCard.IdentityID, "queued")
	if err != nil {
		t.Fatalf("send should not fail hard, got %v", err)
	}
	msgs, err := alice.GetMessages(bobCard.IdentityID, 10, 0)
	if err != nil {
		t.Fatalf("alice get messages failed: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != msgID || msgs[0].Status != "pending" {
		t.Fatal("expected pending message metadata when network is disconnected")
	}
}

func TestServiceSendMessagePublishesNewMessageNotification(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	contactID := "aim1_notify_contact_01"
	if err := svc.AddContact(contactID, "notify-contact"); err != nil {
		t.Fatalf("add contact failed: %v", err)
	}

	_, ch, cancel := svc.SubscribeNotifications(0)
	defer cancel()

	msgID, err := svc.SendMessage(contactID, "hello from cli")
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-ch:
			if evt.Method != "notify.message.new" {
				continue
			}
			payload, ok := evt.Payload.(map[string]any)
			if !ok {
				t.Fatalf("unexpected payload type: %T", evt.Payload)
			}
			if gotContact, _ := payload["contact_id"].(string); gotContact != contactID {
				t.Fatalf("unexpected contact_id: %q", gotContact)
			}
			msgPayload, ok := payload["message"].(models.Message)
			if !ok {
				t.Fatalf("unexpected message payload type: %T", payload["message"])
			}
			if msgPayload.ID != msgID {
				t.Fatalf("unexpected message id in notification: %q", msgPayload.ID)
			}
			if msgPayload.Direction != "out" {
				t.Fatalf("unexpected direction: %q", msgPayload.Direction)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for notify.message.new event")
		}
	}
}

func TestServiceMetricsExposePeerCountQueueAndErrors(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	bobCard, err := bob.SelfContactCard("bob")
	if err != nil {
		t.Fatalf("bob card failed: %v", err)
	}
	if err := svc.AddContactCard(bobCard); err != nil {
		t.Fatalf("add contact failed: %v", err)
	}
	if _, err := svc.SendMessage(bobCard.IdentityID, "queued while disconnected"); err != nil {
		t.Fatalf("send message should not fail hard: %v", err)
	}
	metrics := svc.GetMetrics()
	if metrics.PendingQueueSize < 1 {
		t.Fatalf("expected pending queue size >= 1, got %d", metrics.PendingQueueSize)
	}
	if metrics.ErrorCounters["network"] < 1 {
		t.Fatalf("expected network errors >= 1, got %d", metrics.ErrorCounters["network"])
	}
	if metrics.OperationStats == nil {
		t.Fatal("expected operation_stats to be present")
	}
	if metrics.NetworkMetrics == nil {
		t.Fatal("expected network_metrics to be present")
	}
	if _, ok := metrics.NetworkMetrics["network_state_transitions"]; !ok {
		t.Fatal("expected network_state_transitions metric")
	}
	sendStats, ok := metrics.OperationStats["message.send"]
	if !ok || sendStats.Count < 1 {
		t.Fatal("expected message.send operation stats to be recorded")
	}
	if sendStats.Errors != 0 {
		t.Fatalf("expected message.send errors=0 for queued-success path, got %d", sendStats.Errors)
	}
	if metrics.LastUpdatedAt.IsZero() {
		t.Fatal("expected last_updated_at to be set")
	}
}

func TestServiceOperationErrorsIncrementOnFailedOperation(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	if _, err := svc.GetMessageStatus("missing-message-id"); err == nil {
		t.Fatal("expected get message status to fail")
	}

	metrics := svc.GetMetrics()
	statusStats, ok := metrics.OperationStats["message.status"]
	if !ok {
		t.Fatal("expected message.status operation stats to be recorded")
	}
	if statusStats.Count < 1 {
		t.Fatalf("expected message.status count >= 1, got %d", statusStats.Count)
	}
	if statusStats.Errors < 1 {
		t.Fatalf("expected message.status errors >= 1, got %d", statusStats.Errors)
	}
}

func TestServiceStartupRecoveryProcessesPending(t *testing.T) {
	dir := t.TempDir()
	msgPath := filepath.Join(dir, "messages.json")

	msgStoreA, err := storage.NewPersistentMessageStore(msgPath)
	if err != nil {
		t.Fatalf("new message store A failed: %v", err)
	}
	msgStoreB, err := storage.NewPersistentMessageStore(filepath.Join(dir, "messages-bob.json"))
	if err != nil {
		t.Fatalf("new message store B failed: %v", err)
	}

	alice, err := NewServiceForTesting(waku.DefaultConfig(), contracts.ServiceOptions{
		SessionStore: crypto.NewInMemorySessionStore(),
		MessageStore: msgStoreA,
	})
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	_, mnemonic, err := alice.CreateIdentity("alice-pass")
	if err != nil {
		t.Fatalf("alice create identity failed: %v", err)
	}
	bob, err := NewServiceForTesting(waku.DefaultConfig(), contracts.ServiceOptions{
		SessionStore: crypto.NewInMemorySessionStore(),
		MessageStore: msgStoreB,
	})
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}

	bobCard, err := bob.SelfContactCard("bob")
	if err != nil {
		t.Fatalf("bob card failed: %v", err)
	}
	if err := alice.AddContactCard(bobCard); err != nil {
		t.Fatalf("alice add contact failed: %v", err)
	}
	aliceCard, err := alice.SelfContactCard("alice")
	if err != nil {
		t.Fatalf("alice card failed: %v", err)
	}
	if err := bob.AddContactCard(aliceCard); err != nil {
		t.Fatalf("bob add alice contact failed: %v", err)
	}

	if _, err := alice.SendMessage(bobCard.IdentityID, "recover me"); err != nil {
		t.Fatalf("send disconnected failed: %v", err)
	}

	// Simulate restart with the same persistent message store.
	msgStoreA2, err := storage.NewPersistentMessageStore(msgPath)
	if err != nil {
		t.Fatalf("new message store A2 failed: %v", err)
	}
	alice2, err := NewServiceForTesting(waku.DefaultConfig(), contracts.ServiceOptions{
		SessionStore: crypto.NewInMemorySessionStore(),
		MessageStore: msgStoreA2,
	})
	if err != nil {
		t.Fatalf("new alice2 service failed: %v", err)
	}
	if _, err := alice2.ImportIdentity(mnemonic, "alice-pass"); err != nil {
		t.Fatalf("alice2 import identity failed: %v", err)
	}
	if err := alice2.AddContactCard(bobCard); err != nil {
		t.Fatalf("alice2 add contact failed: %v", err)
	}

	if err := bob.StartNetworking(context.Background()); err != nil {
		t.Fatalf("bob start networking failed: %v", err)
	}
	if err := alice2.StartNetworking(context.Background()); err != nil {
		t.Fatalf("alice2 start networking failed: %v", err)
	}

	aliceIdentity, err := alice2.GetIdentity()
	if err != nil {
		t.Fatalf("alice2 get identity failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		msgs, err := bob.GetMessages(aliceIdentity.ID, 10, 0)
		if err != nil {
			t.Fatalf("bob list messages failed: %v", err)
		}
		if len(msgs) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup recovery did not deliver pending message")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := alice2.StopNetworking(context.Background()); err != nil {
		t.Fatalf("alice2 stop networking failed: %v", err)
	}
	if err := bob.StopNetworking(context.Background()); err != nil {
		t.Fatalf("bob stop networking failed: %v", err)
	}
}

func TestServiceStopNetworkingHonorsTimeout(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := svc.StopNetworking(ctx); err != nil {
		t.Fatalf("stop networking failed: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("stop networking exceeded expected timeout window")
	}
}

func TestServiceNetworkingLifecycleIsIdempotent(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	metricsAfterFirstStart := svc.GetMetrics()
	transitionsAfterFirstStart := metricsAfterFirstStart.NetworkMetrics["network_state_transitions"]

	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("second start failed: %v", err)
	}
	metricsAfterSecondStart := svc.GetMetrics()
	transitionsAfterSecondStart := metricsAfterSecondStart.NetworkMetrics["network_state_transitions"]
	if transitionsAfterSecondStart != transitionsAfterFirstStart {
		t.Fatalf("expected second start to be no-op: %d vs %d", transitionsAfterSecondStart, transitionsAfterFirstStart)
	}

	if err := svc.StopNetworking(context.Background()); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}
	metricsAfterFirstStop := svc.GetMetrics()
	transitionsAfterFirstStop := metricsAfterFirstStop.NetworkMetrics["network_state_transitions"]

	if err := svc.StopNetworking(context.Background()); err != nil {
		t.Fatalf("second stop failed: %v", err)
	}
	metricsAfterSecondStop := svc.GetMetrics()
	transitionsAfterSecondStop := metricsAfterSecondStop.NetworkMetrics["network_state_transitions"]
	if transitionsAfterSecondStop != transitionsAfterFirstStop {
		t.Fatalf("expected second stop to be no-op: %d vs %d", transitionsAfterSecondStop, transitionsAfterFirstStop)
	}
}

func TestServiceDeviceLifecycle(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	devices, err := svc.ListDevices()
	if err != nil {
		t.Fatalf("list devices failed: %v", err)
	}
	if len(devices) < 1 {
		t.Fatal("expected at least primary device")
	}

	added, err := svc.AddDevice("tablet")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}
	if added.ID == "" {
		t.Fatal("added device id is empty")
	}

	rev, err := svc.RevokeDevice(added.ID)
	if err != nil {
		t.Fatalf("revoke device failed: %v", err)
	}
	if rev.DeviceID != added.ID {
		t.Fatal("revocation device id mismatch")
	}

	devices, err = svc.ListDevices()
	if err != nil {
		t.Fatalf("list devices failed: %v", err)
	}
	revoked := false
	for _, d := range devices {
		if d.ID == added.ID {
			revoked = d.IsRevoked
		}
	}
	if !revoked {
		t.Fatal("device should be revoked in list")
	}
}

func TestServiceRevokeDeviceReturnsPartialDeliveryError(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.AddContact("aim1_partial_contact_01", "alice"); err != nil {
		t.Fatalf("add contact #1 failed: %v", err)
	}
	if err := svc.AddContact("aim1_partial_contact_02", "bob"); err != nil {
		t.Fatalf("add contact #2 failed: %v", err)
	}
	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	defer func() { _ = svc.StopNetworking(context.Background()) }()

	svc.SetPublishFailuresForTesting(map[string]error{
		"aim1_partial_contact_02": errors.New("dial failed"),
	})

	added, err := svc.AddDevice("tablet")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}
	_, err = svc.RevokeDevice(added.ID)
	if err == nil {
		t.Fatal("expected partial delivery error")
	}
	attempted, failed, full, ok := DeviceRevocationDeliveryStats(err)
	if !ok {
		t.Fatalf("expected device revocation delivery stats, got %T", err)
	}
	if attempted != 2 || failed != 1 {
		t.Fatalf("unexpected delivery stats: attempted=%d failed=%d", attempted, failed)
	}
	if full {
		t.Fatal("partial error must not be full failure")
	}
}

func TestServiceRevokeDeviceReturnsFullDeliveryError(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.AddContact("aim1_full_contact_01", "alice"); err != nil {
		t.Fatalf("add contact #1 failed: %v", err)
	}
	if err := svc.AddContact("aim1_full_contact_02", "bob"); err != nil {
		t.Fatalf("add contact #2 failed: %v", err)
	}
	if err := svc.StartNetworking(context.Background()); err != nil {
		t.Fatalf("start networking failed: %v", err)
	}
	defer func() { _ = svc.StopNetworking(context.Background()) }()

	svc.SetPublishFailuresForTesting(map[string]error{
		"aim1_full_contact_01": errors.New("link down"),
		"aim1_full_contact_02": errors.New("dial failed"),
	})

	added, err := svc.AddDevice("tablet")
	if err != nil {
		t.Fatalf("add device failed: %v", err)
	}
	_, err = svc.RevokeDevice(added.ID)
	if err == nil {
		t.Fatal("expected full delivery error")
	}
	attempted, failed, full, ok := DeviceRevocationDeliveryStats(err)
	if !ok {
		t.Fatalf("expected device revocation delivery stats, got %T", err)
	}
	if attempted != 2 || failed != 2 {
		t.Fatalf("unexpected delivery stats: attempted=%d failed=%d", attempted, failed)
	}
	if !full {
		t.Fatal("expected full delivery failure")
	}
}

func TestServiceMessageReceiptsDeliveredAndRead(t *testing.T) {
	alice, bob, _, bobCard := newServicePairWithMutualContacts(t)
	startServicePairNetworking(t, alice, bob)

	msgID, err := alice.SendMessage(bobCard.IdentityID, "status-flow")
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	deadlineDelivered := time.Now().Add(3 * time.Second)
	for {
		status, err := alice.GetMessageStatus(msgID)
		if err != nil {
			t.Fatalf("alice get status failed: %v", err)
		}
		if status.Status == "delivered" || status.Status == "read" {
			break
		}
		if time.Now().After(deadlineDelivered) {
			t.Fatalf("timed out waiting for delivered status, current=%s", status.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	aliceIdentity, err := alice.GetIdentity()
	if err != nil {
		t.Fatalf("alice get identity failed: %v", err)
	}
	if _, err := bob.GetMessages(aliceIdentity.ID, 20, 0); err != nil {
		t.Fatalf("bob get messages failed: %v", err)
	}

	deadlineRead := time.Now().Add(3 * time.Second)
	for {
		status, err := alice.GetMessageStatus(msgID)
		if err != nil {
			t.Fatalf("alice get status failed: %v", err)
		}
		if status.Status == "read" {
			break
		}
		if time.Now().After(deadlineRead) {
			t.Fatalf("timed out waiting for read status, current=%s", status.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceOutOfOrderReceiptDoesNotRegressStatus(t *testing.T) {
	alice, bob, _, bobCard := newServicePairWithMutualContacts(t)

	msgID, err := alice.SendMessage(bobCard.IdentityID, "receipt-order")
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()

	deliveredID := "rcpt-delivered"
	deliveredWire, err := signedReceiptWire(bob, deliveredID, "delivered", msgID, bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build delivered receipt failed: %v", err)
	}
	readID := "rcpt-read"
	readWire, err := signedReceiptWire(bob, readID, "read", msgID, bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build read receipt failed: %v", err)
	}

	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        readID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   readWire,
	})
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        deliveredID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   deliveredWire,
	})

	status, err := alice.GetMessageStatus(msgID)
	if err != nil {
		t.Fatalf("alice get status failed: %v", err)
	}
	if status.Status != "read" {
		t.Fatalf("expected read to remain final status, got %s", status.Status)
	}
}

func TestServiceRejectsInboundFromUnverifiedContact(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyContactsOnly)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}
	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()

	wireID := "msg-unverified"
	wireData, err := signedPlainWire(bob, wireID, "hello", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        wireID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	contacts, err := alice.GetContacts()
	if err != nil {
		t.Fatalf("get contacts failed: %v", err)
	}
	if len(contacts) != 0 {
		t.Fatalf("expected no auto-added contacts, got %d", len(contacts))
	}
	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no saved messages from unverified sender, got %d", len(msgs))
	}
}

func TestServiceBlocklistOverridesEveryoneModeForUnknownSender(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()

	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyEveryone)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}
	if _, err := alice.AddToBlocklist(bobID.ID); err != nil {
		t.Fatalf("add to blocklist failed: %v", err)
	}

	wireID := "msg-blocked-everyone"
	wireData, err := signedPlainWire(bob, wireID, "blocked in everyone mode", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        wireID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no saved messages from blocked sender in everyone mode, got %d", len(msgs))
	}
}

func TestServiceRejectsInboundFromBlockedSender(t *testing.T) {
	alice, bob, _, bobCard := newServicePairWithMutualContacts(t)

	if _, err := alice.AddToBlocklist(bobCard.IdentityID); err != nil {
		t.Fatalf("add to blocklist failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()

	wireID := "msg-blocked"
	wireData, err := signedPlainWire(bob, wireID, "should be blocked", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        wireID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no saved messages from blocked sender, got %d", len(msgs))
	}
}

func TestServiceRequestsModeQueuesUnknownSender(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireID := "msg-requests-mode"
	wireData, err := signedPlainWire(bob, wireID, "hello", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        wireID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no saved messages in requests mode until inbox flow, got %d", len(msgs))
	}

	requests, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list message requests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one message request, got %d", len(requests))
	}
	if requests[0].SenderID != bobID.ID {
		t.Fatalf("unexpected request sender: got=%s want=%s", requests[0].SenderID, bobID.ID)
	}
	if requests[0].MessageCount != 1 {
		t.Fatalf("unexpected request message count: got=%d want=1", requests[0].MessageCount)
	}

	thread, err := alice.GetMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("get message request failed: %v", err)
	}
	if thread.Request.SenderID != bobID.ID {
		t.Fatalf("unexpected thread sender: got=%s want=%s", thread.Request.SenderID, bobID.ID)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("expected one message in request thread, got %d", len(thread.Messages))
	}
	if string(thread.Messages[0].Content) != "hello" {
		t.Fatalf("unexpected request message content: got=%q want=%q", string(thread.Messages[0].Content), "hello")
	}
}

func TestServiceAcceptMessageRequestMovesThreadToMainChatAndIsIdempotent(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireData, err := signedPlainWire(bob, "msg-requests-accept-1", "first", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire #1 failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-accept-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})
	wireData2, err := signedPlainWire(bob, "msg-requests-accept-2", "second", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire #2 failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-accept-2",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData2,
	})

	requestsBefore, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests before accept failed: %v", err)
	}
	if len(requestsBefore) != 1 {
		t.Fatalf("expected one request before accept, got %d", len(requestsBefore))
	}

	accepted, err := alice.AcceptMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("accept request failed: %v", err)
	}
	if !accepted {
		t.Fatal("expected accepted=true")
	}

	contacts, err := alice.GetContacts()
	if err != nil {
		t.Fatalf("get contacts failed: %v", err)
	}
	found := false
	for _, c := range contacts {
		if c.ID == bobID.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("accepted sender must be auto-added to contacts: %s", bobID.ID)
	}

	requestsAfter, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests after accept failed: %v", err)
	}
	if len(requestsAfter) != 0 {
		t.Fatalf("expected empty requests inbox after accept, got %d", len(requestsAfter))
	}

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected two moved messages in main chat, got %d", len(msgs))
	}

	acceptedAgain, err := alice.AcceptMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("idempotent accept failed: %v", err)
	}
	if !acceptedAgain {
		t.Fatal("expected accepted=true on idempotent repeat")
	}

	msgsAfterRepeat, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages after repeat failed: %v", err)
	}
	if len(msgsAfterRepeat) != 2 {
		t.Fatalf("idempotent accept must not duplicate moved history, got %d", len(msgsAfterRepeat))
	}
}

func TestServiceDeclineMessageRequestRemovesInboxWithoutAddingContact(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireData, err := signedPlainWire(bob, "msg-requests-decline-1", "first", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-decline-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	requestsBefore, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests before decline failed: %v", err)
	}
	if len(requestsBefore) != 1 {
		t.Fatalf("expected one request before decline, got %d", len(requestsBefore))
	}

	declined, err := alice.DeclineMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("decline request failed: %v", err)
	}
	if !declined {
		t.Fatal("expected declined=true")
	}

	requestsAfter, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests after decline failed: %v", err)
	}
	if len(requestsAfter) != 0 {
		t.Fatalf("expected empty requests inbox after decline, got %d", len(requestsAfter))
	}

	contacts, err := alice.GetContacts()
	if err != nil {
		t.Fatalf("get contacts failed: %v", err)
	}
	for _, c := range contacts {
		if c.ID == bobID.ID {
			t.Fatalf("decline must not auto-add contact: %s", bobID.ID)
		}
	}

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("decline must not move messages to main chat, got %d", len(msgs))
	}

	declinedAgain, err := alice.DeclineMessageRequest(bobID.ID)
	if err != nil {
		t.Fatalf("idempotent decline failed: %v", err)
	}
	if !declinedAgain {
		t.Fatal("idempotent decline must return declined=true")
	}
}

func TestServiceDeclineRequestAllowsFutureRequestsPerPolicy(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	firstWire, err := signedPlainWire(bob, "msg-requests-decline-2", "first", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build first wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-decline-2",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   firstWire,
	})
	if _, err := alice.DeclineMessageRequest(bobID.ID); err != nil {
		t.Fatalf("decline failed: %v", err)
	}

	secondWire, err := signedPlainWire(bob, "msg-requests-decline-3", "second", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build second wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-decline-3",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   secondWire,
	})

	requests, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests failed: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one new request after re-delivery, got %d", len(requests))
	}
	if requests[0].SenderID != bobID.ID {
		t.Fatalf("unexpected sender in renewed request: got=%s want=%s", requests[0].SenderID, bobID.ID)
	}
	if requests[0].MessageCount != 1 {
		t.Fatalf("expected only the new message in renewed request, got=%d", requests[0].MessageCount)
	}
}

func TestServiceBlockSenderRemovesRequestAndOverridesPrivacyModes(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyRequests)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireData, err := signedPlainWire(bob, "msg-requests-block-1", "first", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-block-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	requestsBefore, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests before block failed: %v", err)
	}
	if len(requestsBefore) != 1 {
		t.Fatalf("expected one request before block, got %d", len(requestsBefore))
	}

	result, err := alice.BlockSender(bobID.ID)
	if err != nil {
		t.Fatalf("block sender failed: %v", err)
	}
	if !result.RequestRemoved {
		t.Fatal("expected request_removed=true when blocking sender with pending request")
	}
	if result.ContactExists {
		t.Fatal("expected contact_exists=false for unknown sender request")
	}
	foundBlocked := false
	for _, id := range result.Blocked {
		if id == bobID.ID {
			foundBlocked = true
			break
		}
	}
	if !foundBlocked {
		t.Fatalf("blocked list must include sender id %s", bobID.ID)
	}

	requestsAfter, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests after block failed: %v", err)
	}
	if len(requestsAfter) != 0 {
		t.Fatalf("expected empty requests inbox after block, got %d", len(requestsAfter))
	}

	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyEveryone)); err != nil {
		t.Fatalf("update privacy mode to everyone failed: %v", err)
	}
	wireData2, err := signedPlainWire(bob, "msg-requests-block-2", "second", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build second wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-requests-block-2",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData2,
	})

	requestsAfterSecondInbound, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests after second inbound failed: %v", err)
	}
	if len(requestsAfterSecondInbound) != 0 {
		t.Fatalf("blocked sender must not create new request under everyone mode, got %d", len(requestsAfterSecondInbound))
	}
	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("blocked sender must not create normal chat messages, got %d", len(msgs))
	}
}

func TestServiceEveryoneModeCreatesImmediateChatForUnknownSender(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyEveryone)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireData, err := signedPlainWire(bob, "msg-everyone-unknown-1", "hello everyone", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-everyone-unknown-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	requests, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests failed: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("everyone mode must bypass requests inbox, got %d entries", len(requests))
	}

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("everyone mode must create immediate chat message, got %d", len(msgs))
	}
	if string(msgs[0].Content) != "hello everyone" {
		t.Fatalf("unexpected message content: got=%q want=%q", string(msgs[0].Content), "hello everyone")
	}

	contacts, err := alice.GetContacts()
	if err != nil {
		t.Fatalf("get contacts failed: %v", err)
	}
	found := false
	for _, c := range contacts {
		if c.ID == bobID.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("everyone mode inbound must establish contact context for sender: %s", bobID.ID)
	}
}

func TestServiceContactsOnlyModeDropsUnknownSenderInbound(t *testing.T) {
	alice, err := NewService()
	if err != nil {
		t.Fatalf("new alice service failed: %v", err)
	}
	bob, err := NewService()
	if err != nil {
		t.Fatalf("new bob service failed: %v", err)
	}
	if _, err := alice.UpdatePrivacySettings(string(app.MessagePrivacyContactsOnly)); err != nil {
		t.Fatalf("update privacy mode failed: %v", err)
	}

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	wireData, err := signedPlainWire(bob, "msg-contacts-only-unknown-1", "must be dropped", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        "msg-contacts-only-unknown-1",
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	requests, err := alice.ListMessageRequests()
	if err != nil {
		t.Fatalf("list requests failed: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("contacts_only mode must not create requests inbox entries, got %d", len(requests))
	}

	msgs, err := alice.GetMessages(bobID.ID, 10, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("contacts_only mode must drop unknown inbound messages, got %d", len(msgs))
	}
}

func TestServiceBlocklistOverridesAllPrivacyModes(t *testing.T) {
	cases := []struct {
		name string
		mode app.MessagePrivacyMode
	}{
		{name: "contacts_only", mode: app.MessagePrivacyContactsOnly},
		{name: "requests", mode: app.MessagePrivacyRequests},
		{name: "everyone", mode: app.MessagePrivacyEveryone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alice, err := NewService()
			if err != nil {
				t.Fatalf("new alice service failed: %v", err)
			}
			bob, err := NewService()
			if err != nil {
				t.Fatalf("new bob service failed: %v", err)
			}
			if _, err := alice.UpdatePrivacySettings(string(tc.mode)); err != nil {
				t.Fatalf("update privacy mode failed: %v", err)
			}

			aliceID, _ := alice.GetIdentity()
			bobID, _ := bob.GetIdentity()
			if _, err := alice.BlockSender(bobID.ID); err != nil {
				t.Fatalf("block sender failed: %v", err)
			}

			wireData, err := signedPlainWire(bob, "msg-blocklist-"+tc.name, "blocked inbound", bobID.ID, aliceID.ID)
			if err != nil {
				t.Fatalf("build wire failed: %v", err)
			}
			alice.handleIncomingPrivateMessage(waku.PrivateMessage{
				ID:        "msg-blocklist-" + tc.name,
				SenderID:  bobID.ID,
				Recipient: aliceID.ID,
				Payload:   wireData,
			})

			requests, err := alice.ListMessageRequests()
			if err != nil {
				t.Fatalf("list requests failed: %v", err)
			}
			if len(requests) != 0 {
				t.Fatalf("blocked sender must not create request inbox entries in %s mode, got %d", tc.mode, len(requests))
			}

			msgs, err := alice.GetMessages(bobID.ID, 10, 0)
			if err != nil {
				t.Fatalf("get messages failed: %v", err)
			}
			if len(msgs) != 0 {
				t.Fatalf("blocked sender must not create chat messages in %s mode, got %d", tc.mode, len(msgs))
			}
		})
	}
}

func TestServiceRejectsInboundMessageIDConflict(t *testing.T) {
	alice, bob, _, _ := newServicePairWithMutualContacts(t)

	aliceID, _ := alice.GetIdentity()
	bobID, _ := bob.GetIdentity()
	const dupID = "msg-dup-conflict"
	original := models.Message{
		ID:          dupID,
		ContactID:   bobID.ID,
		Content:     []byte("original"),
		Timestamp:   time.Now().UTC(),
		Direction:   "out",
		Status:      "sent",
		ContentType: "text",
	}
	if err := alice.SaveMessageForTesting(original); err != nil {
		t.Fatalf("seed message failed: %v", err)
	}

	wireData, err := signedPlainWire(bob, dupID, "malicious overwrite", bobID.ID, aliceID.ID)
	if err != nil {
		t.Fatalf("build wire failed: %v", err)
	}
	alice.handleIncomingPrivateMessage(waku.PrivateMessage{
		ID:        dupID,
		SenderID:  bobID.ID,
		Recipient: aliceID.ID,
		Payload:   wireData,
	})

	got, ok := alice.GetMessageForTesting(dupID)
	if !ok {
		t.Fatal("seed message disappeared after conflict")
	}
	if string(got.Content) != "original" || got.Direction != "out" {
		t.Fatalf("message was overwritten on id conflict: %+v", got)
	}
}

func TestServicePutAttachmentRejectsOversizedPayload(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	raw := strings.Repeat("a", app.MaxAttachmentBytes+1)
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))

	_, err = svc.PutAttachment("big.bin", "application/octet-stream", encoded)
	if err == nil {
		t.Fatal("expected oversized attachment to fail")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceGetMessagesCallsListOnce(t *testing.T) {
	store := &countingMessageStore{
		items: []models.Message{
			{
				ID:          "m1",
				ContactID:   "aim1_contact_1",
				Content:     []byte("hello"),
				Timestamp:   time.Now().UTC(),
				Direction:   "out",
				Status:      "sent",
				ContentType: "text",
			},
		},
	}
	svc, err := NewServiceForTesting(waku.DefaultConfig(), contracts.ServiceOptions{
		SessionStore: crypto.NewInMemorySessionStore(),
		MessageStore: store,
		Logger:       app.DefaultLogger(),
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}

	msgs, err := svc.GetMessages("aim1_contact_1", 20, 0)
	if err != nil {
		t.Fatalf("get messages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %d", len(msgs))
	}
	if store.listCalls != 1 {
		t.Fatalf("expected one list call, got %d", store.listCalls)
	}
}

func TestServiceStartNetworkingConcurrentStartsTransportOnce(t *testing.T) {
	svc, err := NewService()
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	node := newBlockingTransportNode()
	prev := svc.SetTransportNodeForTesting(node)
	defer svc.SetTransportNodeForTesting(prev)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			errCh <- svc.StartNetworking(context.Background())
		}()
	}

	select {
	case <-node.startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("transport start was not called")
	}

	if got := node.StartCalls(); got != 1 {
		t.Fatalf("expected one in-flight start call, got %d", got)
	}
	close(node.allowStartReturn)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent start failed: %v", err)
		}
	}
	if got := node.StartCalls(); got != 1 {
		t.Fatalf("expected transport start to be called once, got %d", got)
	}
	if err := svc.StopNetworking(context.Background()); err != nil {
		t.Fatalf("stop networking failed: %v", err)
	}
}

func signedReceiptWire(sender *Service, wireID, status, messageID, senderID, recipientID string) ([]byte, error) {
	return sender.BuildSignedReceiptWireForTesting(wireID, status, messageID, senderID, recipientID)
}

func signedPlainWire(sender *Service, wireID, content, senderID, recipientID string) ([]byte, error) {
	return sender.BuildSignedPlainWireForTesting(wireID, content, senderID, recipientID)
}

type countingMessageStore struct {
	items     []models.Message
	listCalls int
}

func (s *countingMessageStore) SaveMessage(_ models.Message) error { return nil }

func (s *countingMessageStore) Snapshot() (map[string]models.Message, map[string]storage.PendingMessage) {
	return map[string]models.Message{}, map[string]storage.PendingMessage{}
}

func (s *countingMessageStore) AddOrUpdatePending(_ models.Message, _ int, _ time.Time, _ string) error {
	return nil
}

func (s *countingMessageStore) RemovePending(_ string) error { return nil }

func (s *countingMessageStore) UpdateMessageStatus(_, _ string) (bool, error) {
	return true, nil
}

func (s *countingMessageStore) GetMessage(messageID string) (models.Message, bool) {
	for _, item := range s.items {
		if item.ID == messageID {
			return item, true
		}
	}
	return models.Message{}, false
}

func (s *countingMessageStore) UpdateMessageContent(_ string, _ []byte, _ string) (models.Message, bool, error) {
	return models.Message{}, false, nil
}

func (s *countingMessageStore) DeleteMessage(_, _ string) (bool, error) { return true, nil }

func (s *countingMessageStore) ClearMessages(_ string) (int, error) { return 0, nil }

func (s *countingMessageStore) ListMessages(contactID string, _, _ int) []models.Message {
	s.listCalls++
	out := make([]models.Message, 0, len(s.items))
	for _, item := range s.items {
		if item.ContactID == contactID {
			out = append(out, item)
		}
	}
	return out
}

func (s *countingMessageStore) PendingCount() int { return 0 }

func (s *countingMessageStore) DuePending(_ time.Time) []storage.PendingMessage {
	return nil
}

type blockingTransportNode struct {
	startCalls       int32
	startEntered     chan struct{}
	allowStartReturn chan struct{}
}

func newBlockingTransportNode() *blockingTransportNode {
	return &blockingTransportNode{
		startEntered:     make(chan struct{}),
		allowStartReturn: make(chan struct{}),
	}
}

func (n *blockingTransportNode) Start(ctx context.Context) error {
	if atomic.AddInt32(&n.startCalls, 1) == 1 {
		close(n.startEntered)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.allowStartReturn:
		return nil
	}
}

func (n *blockingTransportNode) Stop(_ context.Context) error { return nil }

func (n *blockingTransportNode) Status() waku.Status {
	return waku.Status{State: waku.StateConnected, PeerCount: 1, LastSync: time.Now()}
}

func (n *blockingTransportNode) SetIdentity(_ string) {}

func (n *blockingTransportNode) SubscribePrivate(_ func(waku.PrivateMessage)) error { return nil }

func (n *blockingTransportNode) PublishPrivate(_ context.Context, _ waku.PrivateMessage) error {
	return nil
}

func (n *blockingTransportNode) FetchPrivateSince(_ context.Context, _ string, _ time.Time, _ int) ([]waku.PrivateMessage, error) {
	return nil, nil
}

func (n *blockingTransportNode) ListenAddresses() []string { return nil }

func (n *blockingTransportNode) NetworkMetrics() map[string]int { return map[string]int{} }

func (n *blockingTransportNode) StartCalls() int {
	return int(atomic.LoadInt32(&n.startCalls))
}
