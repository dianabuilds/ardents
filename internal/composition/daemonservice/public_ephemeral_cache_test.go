package daemonservice

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"aim-chat/go-backend/internal/storage"
	"aim-chat/go-backend/pkg/models"
)

func TestPublicFetchedBlobIsNotPersistedWhenPublicStoreDisabled(t *testing.T) {
	cfg := newMockConfig()
	registry := newBlobProviderRegistry()
	baseDir := t.TempDir()
	senderDir := filepath.Join(baseDir, "sender")
	receiverDir := filepath.Join(baseDir, "receiver")

	sender, err := NewServiceForDaemonWithDataDir(cfg, senderDir)
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	receiver, err := NewServiceForDaemonWithDataDir(cfg, receiverDir)
	if err != nil {
		t.Fatalf("new receiver: %v", err)
	}
	useSharedBlobProviders(registry, sender, receiver)
	if _, _, err := sender.CreateIdentity("sender-pass"); err != nil {
		t.Fatalf("create sender identity: %v", err)
	}
	if _, _, err := receiver.CreateIdentity("receiver-pass"); err != nil {
		t.Fatalf("create receiver identity: %v", err)
	}
	storeOff := false
	if _, err := receiver.UpdateNodePolicies(models.NodePoliciesPatch{
		Public: &models.NodePublicPolicyPatch{StoreEnabled: &storeOff},
	}); err != nil {
		t.Fatalf("set receiver public store policy: %v", err)
	}
	startBlobNetworking(t, sender, receiver)
	cleanupBlobNetworking(t, sender, receiver)

	payload := make([]byte, 128*1024)
	meta, err := sender.PutAttachment("remote.bin", "application/octet-stream", base64.StdEncoding.EncodeToString(payload))
	if err != nil {
		t.Fatalf("put sender attachment: %v", err)
	}
	if _, _, err := receiver.GetAttachment(meta.ID); err != nil {
		t.Fatalf("receiver fetch attachment: %v", err)
	}
	if lister, ok := receiver.attachmentStore.(interface {
		ListMetas() []models.AttachmentMeta
	}); ok {
		for _, stored := range lister.ListMetas() {
			if stored.ID == meta.ID {
				t.Fatalf("fetched blob must not be persisted in durable store when public_store is disabled")
			}
		}
	}

	stopBlobNetworkingNow(t, receiver, "receiver")
	stopBlobNetworkingNow(t, sender, "sender")

	reloaded, err := NewServiceForDaemonWithDataDir(cfg, receiverDir)
	if err != nil {
		t.Fatalf("reload receiver: %v", err)
	}
	if _, _, err := reloaded.identityCore.GetAttachment(meta.ID); !errors.Is(err, storage.ErrAttachmentNotFound) {
		t.Fatalf("expected no durable fetched blob after restart, got err=%v", err)
	}
}

func TestPublicEphemeralBlobCacheExpiresByTTL(t *testing.T) {
	cache := newPublicEphemeralBlobCache(32, 1)
	now := time.Now().UTC()
	meta := models.AttachmentMeta{
		ID:       "att1_ephemeral",
		MimeType: "application/octet-stream",
		Class:    "file",
	}
	if ok := cache.Put(meta, []byte("hello"), now); !ok {
		t.Fatal("put into ephemeral cache must succeed")
	}
	if _, _, ok := cache.Get(meta.ID, now.Add(30*time.Second)); !ok {
		t.Fatal("cache entry must exist before TTL expiry")
	}
	cache.PurgeExpired(now.Add(2 * time.Minute))
	if _, _, ok := cache.Get(meta.ID, now.Add(2*time.Minute)); ok {
		t.Fatal("cache entry must be evicted after TTL expiry")
	}
}
