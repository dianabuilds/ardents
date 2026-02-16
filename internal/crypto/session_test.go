package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aim-chat/go-backend/internal/securestore"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

func TestInitSessionRejectsInvalidPeerKey(t *testing.T) {
	m := NewSessionManager(NewInMemorySessionStore())
	if _, err := m.InitSession("aim1local", "aim1contact", []byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for invalid peer key length")
	}
}

func TestInitSessionDeterministicIDAndStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	store := NewFileSessionStore(path)

	manager1 := NewSessionManager(store)
	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(i + 1)
	}
	state1, err := manager1.InitSession("aim1local", "aim1contact", peerKey)
	if err != nil {
		t.Fatalf("init session failed: %v", err)
	}
	if state1.SessionID == "" {
		t.Fatal("session id must not be empty")
	}

	// Simulate restart: new manager with the same file store.
	manager2 := NewSessionManager(NewFileSessionStore(path))
	state2, ok, err := manager2.GetSession("aim1contact")
	if err != nil {
		t.Fatalf("get session after restart failed: %v", err)
	}
	if !ok {
		t.Fatal("expected session to exist after restart")
	}
	if state1.SessionID != state2.SessionID {
		t.Fatalf("session id must survive restart: %s != %s", state1.SessionID, state2.SessionID)
	}
}

func TestEncryptedFileSessionStoreTamperFailsAuth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.enc")
	store := NewEncryptedFileSessionStore(path, "pass")
	manager := NewSessionManager(store)

	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(i + 1)
	}
	if _, err := manager.InitSession("aim1local", "aim1contact", peerKey); err != nil {
		t.Fatalf("init session failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read encrypted sessions failed: %v", err)
	}
	data[len(data)-4] ^= 0xAB
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write tampered sessions failed: %v", err)
	}

	_, _, err = NewEncryptedFileSessionStore(path, "pass").Get("aim1contact")
	if !errors.Is(err, securestore.ErrAuthFailed) && !errors.Is(err, securestore.ErrInvalid) {
		t.Fatalf("expected ErrAuthFailed or ErrInvalid, got %v", err)
	}
}

func TestEncryptedFileSessionStoreCreatesPrivateDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secure", "sessions.enc")
	store := NewEncryptedFileSessionStore(path, "pass")
	manager := NewSessionManager(store)

	peerKey := make([]byte, 32)
	for i := range peerKey {
		peerKey[i] = byte(i + 1)
	}
	if _, err := manager.InitSession("aim1local", "aim1contact-private", peerKey); err != nil {
		t.Fatalf("init session failed: %v", err)
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir failed: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("expected dir perm 0700, got %04o", info.Mode().Perm())
	}
}

func TestEnvelopeValidationAndReplayGuard(t *testing.T) {
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	env := MessageEnvelope{
		Version:       1,
		SessionID:     "sess1_abc",
		MessageID:     "msg1",
		RatchetPubKey: []byte{1},
		ChainIndex:    0,
		PreviousCount: 0,
		Nonce:         nonce,
		Ciphertext:    []byte{1},
	}
	if err := ValidateEnvelope(env); err != nil {
		t.Fatalf("expected valid envelope, got %v", err)
	}

	guard := NewReplayGuard()
	if guard.Seen("sess1_abc", "msg1") {
		t.Fatal("first message must not be seen")
	}
	if !guard.Seen("sess1_abc", "msg1") {
		t.Fatal("second message should be detected as replay")
	}
}

func TestRatchetEncryptDecryptAndReplay(t *testing.T) {
	alice := NewSessionManager(NewInMemorySessionStore())
	bob := NewSessionManager(NewInMemorySessionStore())
	peer := make([]byte, 32)
	for i := range peer {
		peer[i] = byte(i + 1)
	}
	if _, err := alice.InitSession("aim1alice", "aim1bob", peer); err != nil {
		t.Fatalf("alice init session failed: %v", err)
	}
	if _, err := bob.InitSession("aim1bob", "aim1alice", peer); err != nil {
		t.Fatalf("bob init session failed: %v", err)
	}

	env, err := alice.Encrypt("aim1bob", []byte("secret text"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	plain, err := bob.Decrypt("aim1alice", env)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(plain) != "secret text" {
		t.Fatalf("unexpected plaintext: %s", string(plain))
	}

	if _, err := bob.Decrypt("aim1alice", env); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("expected replay detection, got %v", err)
	}
}

func TestRatchetTamperedCiphertextFails(t *testing.T) {
	alice, bob, _ := newPairedSessionManagers(t, 100)
	env, err := alice.Encrypt("aim1bob", []byte("hello"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	env.Ciphertext[0] ^= 0xFF
	if _, err := bob.Decrypt("aim1alice", env); err == nil {
		t.Fatal("expected tampered ciphertext to fail")
	}
}

func TestRatchetDecryptOutOfOrderWithSkippedKeys(t *testing.T) {
	alice := NewSessionManager(NewInMemorySessionStore())
	bob := NewSessionManager(NewInMemorySessionStore())
	peer := make([]byte, 32)
	for i := range peer {
		peer[i] = byte(50 + i)
	}
	if _, err := alice.InitSession("aim1alice", "aim1bob", peer); err != nil {
		t.Fatalf("alice init session failed: %v", err)
	}
	if _, err := bob.InitSession("aim1bob", "aim1alice", peer); err != nil {
		t.Fatalf("bob init session failed: %v", err)
	}

	env1, err := alice.Encrypt("aim1bob", []byte("m1"))
	if err != nil {
		t.Fatalf("encrypt m1 failed: %v", err)
	}
	env2, err := alice.Encrypt("aim1bob", []byte("m2"))
	if err != nil {
		t.Fatalf("encrypt m2 failed: %v", err)
	}

	plain2, err := bob.Decrypt("aim1alice", env2)
	if err != nil {
		t.Fatalf("decrypt m2 first failed: %v", err)
	}
	if string(plain2) != "m2" {
		t.Fatalf("unexpected plaintext for m2: %s", string(plain2))
	}

	plain1, err := bob.Decrypt("aim1alice", env1)
	if err != nil {
		t.Fatalf("decrypt skipped m1 failed: %v", err)
	}
	if string(plain1) != "m1" {
		t.Fatalf("unexpected plaintext for m1: %s", string(plain1))
	}
}

func TestRatchetRejectsExcessiveGap(t *testing.T) {
	alice, bob, _ := newPairedSessionManagers(t, 80)

	env, err := alice.Encrypt("aim1bob", []byte("hello"))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	env.ChainIndex = maxSkippedChainGap + 1
	if _, err := bob.Decrypt("aim1alice", env); !errors.Is(err, ErrInvalidChainIndex) {
		t.Fatalf("expected ErrInvalidChainIndex, got %v", err)
	}
}

func TestRatchetSkippedKeysSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	alice := NewSessionManager(NewInMemorySessionStore())
	bobStore := NewFileSessionStore(path)
	bob := NewSessionManager(bobStore)

	peer := make([]byte, 32)
	for i := range peer {
		peer[i] = byte(120 + i)
	}
	if _, err := alice.InitSession("aim1alice", "aim1bob", peer); err != nil {
		t.Fatalf("alice init session failed: %v", err)
	}
	if _, err := bob.InitSession("aim1bob", "aim1alice", peer); err != nil {
		t.Fatalf("bob init session failed: %v", err)
	}

	env1, err := alice.Encrypt("aim1bob", []byte("first"))
	if err != nil {
		t.Fatalf("encrypt first failed: %v", err)
	}
	env2, err := alice.Encrypt("aim1bob", []byte("second"))
	if err != nil {
		t.Fatalf("encrypt second failed: %v", err)
	}

	if _, err := bob.Decrypt("aim1alice", env2); err != nil {
		t.Fatalf("decrypt second first failed: %v", err)
	}

	// Simulate restart and ensure skipped message keys are restored from the persisted state.
	bobAfterRestart := NewSessionManager(NewFileSessionStore(path))
	plain1, err := bobAfterRestart.Decrypt("aim1alice", env1)
	if err != nil {
		t.Fatalf("decrypt first after restart failed: %v", err)
	}
	if string(plain1) != "first" {
		t.Fatalf("unexpected plaintext after restart: %s", string(plain1))
	}
}

func TestX3DHSharedSecretMatches(t *testing.T) {
	aliceIKPriv := random32(t)
	aliceEKPriv := random32(t)
	bobIKPriv := random32(t)
	bobSPKPriv := random32(t)
	bobOPKPriv := random32(t)

	aliceIKPub := x25519Pub(t, aliceIKPriv)
	aliceEKPub := x25519Pub(t, aliceEKPriv)
	bobIKPub := x25519Pub(t, bobIKPriv)
	bobSPKPub := x25519Pub(t, bobSPKPriv)
	bobOPKPub := x25519Pub(t, bobOPKPriv)

	s1, err := X3DHInitiatorSharedSecret(aliceIKPriv, aliceEKPriv, bobIKPub, bobSPKPub, bobOPKPub)
	if err != nil {
		t.Fatalf("initiator secret failed: %v", err)
	}
	s2, err := X3DHResponderSharedSecret(bobIKPriv, bobSPKPriv, bobOPKPriv, aliceIKPub, aliceEKPub, true)
	if err != nil {
		t.Fatalf("responder secret failed: %v", err)
	}
	if !bytes.Equal(s1, s2) {
		t.Fatal("x3dh shared secrets must match")
	}
}

func random32(t *testing.T) []byte {
	t.Helper()
	out := make([]byte, 32)
	if _, err := rand.Read(out); err != nil {
		t.Fatalf("rand failed: %v", err)
	}
	return out
}

func x25519Pub(t *testing.T, priv []byte) []byte {
	t.Helper()
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("x25519 pub failed: %v", err)
	}
	return pub
}

func newPairedSessionManagers(t *testing.T, peerOffset byte) (*SessionManager, *SessionManager, []byte) {
	t.Helper()
	alice := NewSessionManager(NewInMemorySessionStore())
	bob := NewSessionManager(NewInMemorySessionStore())
	peer := make([]byte, 32)
	for i := range peer {
		peer[i] = byte(int(peerOffset) + i)
	}
	if _, err := alice.InitSession("aim1alice", "aim1bob", peer); err != nil {
		t.Fatalf("alice init session failed: %v", err)
	}
	if _, err := bob.InitSession("aim1bob", "aim1alice", peer); err != nil {
		t.Fatalf("bob init session failed: %v", err)
	}
	return alice, bob, peer
}
