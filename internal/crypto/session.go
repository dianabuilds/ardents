package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

var (
	ErrInvalidPeerKey    = errors.New("invalid peer key")
	ErrInvalidContact    = errors.New("invalid contact id")
	ErrSessionNotFound   = errors.New("session not found")
	ErrReplayDetected    = errors.New("replay detected")
	ErrInvalidChainIndex = errors.New("invalid chain index")
)

const (
	maxSeenMessageIDs    = 1024
	maxSkippedChainGap   = 512
	maxSkippedMessageKey = 2048
)

type SessionState struct {
	SessionID      string            `json:"session_id"`
	ContactID      string            `json:"contact_id"`
	PeerPublicKey  []byte            `json:"peer_public_key"`
	RootKey        []byte            `json:"root_key"`
	SendChainKey   []byte            `json:"send_chain_key"`
	RecvChainKey   []byte            `json:"recv_chain_key"`
	SendChainIndex uint64            `json:"send_chain_index"`
	RecvChainIndex uint64            `json:"recv_chain_index"`
	SeenMessageIDs []string          `json:"seen_message_ids"`
	SkippedKeys    map[uint64][]byte `json:"skipped_keys"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type SessionStore interface {
	Save(state SessionState) error
	Get(contactID string) (SessionState, bool, error)
	All() ([]SessionState, error)
}

type SessionManager struct {
	store SessionStore
}

type X3DHKeyBundle struct {
	IdentityPrivate  []byte
	SignedPrePrivate []byte
	OneTimePrivate   []byte
	IdentityPublic   []byte
	SignedPrePublic  []byte
	OneTimePublic    []byte
}

func NewSessionManager(store SessionStore) *SessionManager {
	return &SessionManager{store: store}
}

func (m *SessionManager) InitSession(localIdentityID, contactID string, peerPublicKey []byte) (SessionState, error) {
	if contactID == "" {
		return SessionState{}, ErrInvalidContact
	}
	if len(peerPublicKey) != 32 {
		return SessionState{}, ErrInvalidPeerKey
	}

	rootKey := deriveRootKey(localIdentityID, contactID, peerPublicKey)
	sendCK, recvCK := deriveInitialChainKeys(rootKey, localIdentityID, contactID)
	id := buildSessionID(localIdentityID, contactID, peerPublicKey)

	now := time.Now()
	state := SessionState{
		SessionID:      id,
		ContactID:      contactID,
		PeerPublicKey:  append([]byte(nil), peerPublicKey...),
		RootKey:        rootKey,
		SendChainKey:   sendCK,
		RecvChainKey:   recvCK,
		SendChainIndex: 0,
		RecvChainIndex: 0,
		SeenMessageIDs: []string{},
		SkippedKeys:    map[uint64][]byte{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := m.store.Save(state); err != nil {
		return SessionState{}, err
	}
	return state, nil
}

func (m *SessionManager) GetSession(contactID string) (SessionState, bool, error) {
	return m.store.Get(contactID)
}

func (m *SessionManager) Snapshot() ([]SessionState, error) {
	return m.store.All()
}

func (m *SessionManager) Wipe() error {
	if m == nil || m.store == nil {
		return nil
	}
	if wiper, ok := m.store.(interface{ Wipe() error }); ok {
		return wiper.Wipe()
	}
	return nil
}

func (m *SessionManager) SetPersistenceEnabled(enabled bool) {
	if m == nil || m.store == nil {
		return
	}
	if setter, ok := m.store.(interface{ SetPersistenceEnabled(bool) }); ok {
		setter.SetPersistenceEnabled(enabled)
	}
}

func (m *SessionManager) Encrypt(contactID string, plaintext []byte) (MessageEnvelope, error) {
	state, ok, err := m.store.Get(contactID)
	if err != nil {
		return MessageEnvelope{}, err
	}
	if !ok {
		return MessageEnvelope{}, ErrSessionNotFound
	}

	messageID := fmt.Sprintf("dr_%d_%d", time.Now().UnixNano(), state.SendChainIndex)
	msgKey, nextChainKey := deriveMessageKey(state.SendChainKey, state.SendChainIndex)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return MessageEnvelope{}, err
	}

	aead, err := chacha20poly1305.NewX(msgKey)
	if err != nil {
		return MessageEnvelope{}, err
	}
	ad := envelopeAAD(state.SessionID, messageID, state.SendChainIndex)
	ciphertext := aead.Seal(nil, nonce, plaintext, ad)

	env := MessageEnvelope{
		Version:       1,
		SessionID:     state.SessionID,
		MessageID:     messageID,
		RatchetPubKey: deriveRatchetPubKey(state.RootKey, state.SendChainIndex),
		ChainIndex:    state.SendChainIndex,
		PreviousCount: state.RecvChainIndex,
		Nonce:         nonce,
		Ciphertext:    ciphertext,
		SentAt:        time.Now().UTC(),
	}

	state.SendChainIndex++
	state.SendChainKey = nextChainKey
	state.UpdatedAt = time.Now().UTC()
	if err := m.store.Save(state); err != nil {
		return MessageEnvelope{}, err
	}
	return env, nil
}

func (m *SessionManager) Decrypt(contactID string, env MessageEnvelope) ([]byte, error) {
	if err := ValidateEnvelope(env); err != nil {
		return nil, err
	}

	state, ok, err := m.store.Get(contactID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSessionNotFound
	}
	if env.SessionID != state.SessionID {
		return nil, errors.New("session mismatch")
	}
	if seen(state.SeenMessageIDs, env.MessageID) {
		return nil, ErrReplayDetected
	}
	if state.SkippedKeys == nil {
		state.SkippedKeys = map[uint64][]byte{}
	}

	// Out-of-order message can be decrypted with a previously derived skipped key.
	if skippedKey, ok := state.SkippedKeys[env.ChainIndex]; ok {
		aead, err := chacha20poly1305.NewX(skippedKey)
		if err != nil {
			return nil, err
		}
		ad := envelopeAAD(state.SessionID, env.MessageID, env.ChainIndex)
		plaintext, err := aead.Open(nil, env.Nonce, env.Ciphertext, ad)
		if err != nil {
			return nil, err
		}
		delete(state.SkippedKeys, env.ChainIndex)
		state.SeenMessageIDs = appendSeen(state.SeenMessageIDs, env.MessageID, maxSeenMessageIDs)
		state.UpdatedAt = time.Now().UTC()
		if err := m.store.Save(state); err != nil {
			return nil, err
		}
		return plaintext, nil
	}
	if env.ChainIndex < state.RecvChainIndex {
		return nil, ErrInvalidChainIndex
	}
	if env.ChainIndex-state.RecvChainIndex > maxSkippedChainGap {
		return nil, ErrInvalidChainIndex
	}

	chainKey := state.RecvChainKey
	index := state.RecvChainIndex
	for index < env.ChainIndex {
		skippedMsgKey, nextChainKey := deriveMessageKey(chainKey, index)
		state.SkippedKeys[index] = skippedMsgKey
		chainKey = nextChainKey
		index++
	}
	pruneSkippedKeys(state.SkippedKeys, state.RecvChainIndex, maxSkippedMessageKey)
	msgKey, nextChainKey := deriveMessageKey(chainKey, index)

	aead, err := chacha20poly1305.NewX(msgKey)
	if err != nil {
		return nil, err
	}
	ad := envelopeAAD(state.SessionID, env.MessageID, env.ChainIndex)
	plaintext, err := aead.Open(nil, env.Nonce, env.Ciphertext, ad)
	if err != nil {
		return nil, err
	}

	state.RecvChainKey = nextChainKey
	state.RecvChainIndex = env.ChainIndex + 1
	state.SeenMessageIDs = appendSeen(state.SeenMessageIDs, env.MessageID, maxSeenMessageIDs)
	pruneSkippedKeys(state.SkippedKeys, state.RecvChainIndex, maxSkippedMessageKey)
	state.UpdatedAt = time.Now().UTC()
	if err := m.store.Save(state); err != nil {
		return nil, err
	}
	return plaintext, nil
}

type MessageEnvelope struct {
	Version       uint8     `json:"version"`
	SessionID     string    `json:"session_id"`
	MessageID     string    `json:"message_id"`
	RatchetPubKey []byte    `json:"ratchet_pub_key"`
	ChainIndex    uint64    `json:"chain_index"`
	PreviousCount uint64    `json:"previous_count"`
	Nonce         []byte    `json:"nonce"`
	Ciphertext    []byte    `json:"ciphertext"`
	SentAt        time.Time `json:"sent_at"`
}

func ValidateEnvelope(env MessageEnvelope) error {
	if env.Version == 0 {
		return errors.New("invalid version")
	}
	if env.SessionID == "" || env.MessageID == "" {
		return errors.New("missing identifiers")
	}
	if len(env.RatchetPubKey) == 0 || len(env.Nonce) != chacha20poly1305.NonceSizeX || len(env.Ciphertext) == 0 {
		return errors.New("invalid envelope payload")
	}
	return nil
}

type ReplayGuard struct {
	seen map[string]struct{}
}

func NewReplayGuard() *ReplayGuard {
	return &ReplayGuard{seen: make(map[string]struct{})}
}

func (g *ReplayGuard) Seen(sessionID, messageID string) bool {
	key := fmt.Sprintf("%s:%s", sessionID, messageID)
	if _, ok := g.seen[key]; ok {
		return true
	}
	g.seen[key] = struct{}{}
	return false
}

// X3DHInitiatorSharedSecret derives a shared secret for initiator side.
func X3DHInitiatorSharedSecret(ikPriv, ekPriv, peerIKPub, peerSPKPub, peerOPKPub []byte) ([]byte, error) {
	material, err := baseX3DHMaterial(
		[][]byte{ikPriv, ekPriv, peerIKPub, peerSPKPub},
		dhPair{priv: ikPriv, pub: peerSPKPub},
		dhPair{priv: ekPriv, pub: peerIKPub},
		dhPair{priv: ekPriv, pub: peerSPKPub},
	)
	if err != nil {
		return nil, err
	}
	if len(peerOPKPub) == 32 {
		dh4, err := curve25519.X25519(ekPriv, peerOPKPub)
		if err != nil {
			return nil, err
		}
		material = append(material, dh4...)
	}
	return kdf32(material, []byte("aim/x3dh/v1")), nil
}

// X3DHResponderSharedSecret derives a shared secret for responder side.
func X3DHResponderSharedSecret(ikPriv, spkPriv, opkPriv, peerIKPub, peerEKPub []byte, useOPK bool) ([]byte, error) {
	material, err := baseX3DHMaterial(
		[][]byte{ikPriv, spkPriv, peerIKPub, peerEKPub},
		dhPair{priv: spkPriv, pub: peerIKPub},
		dhPair{priv: ikPriv, pub: peerEKPub},
		dhPair{priv: spkPriv, pub: peerEKPub},
	)
	if err != nil {
		return nil, err
	}
	if useOPK {
		if len(opkPriv) != 32 {
			return nil, ErrInvalidPeerKey
		}
		dh4, err := curve25519.X25519(opkPriv, peerEKPub)
		if err != nil {
			return nil, err
		}
		material = append(material, dh4...)
	}
	return kdf32(material, []byte("aim/x3dh/v1")), nil
}

func buildSessionID(localIdentityID, contactID string, peerPublicKey []byte) string {
	idA, idB := normalizeIDs(localIdentityID, contactID)
	h := sha256.Sum256(append([]byte(idA+":"+idB+":"), peerPublicKey...))
	return "sess1_" + hex.EncodeToString(h[:16])
}

func deriveRootKey(localIdentityID, contactID string, peerPublicKey []byte) []byte {
	idA, idB := normalizeIDs(localIdentityID, contactID)
	salt := []byte(idA + ":" + idB)
	input := append([]byte(nil), peerPublicKey...)
	return kdf32(input, append([]byte("aim/session/root/v1|"), salt...))
}

func deriveInitialChainKeys(rootKey []byte, localID, contactID string) ([]byte, []byte) {
	a2b := kdf32(rootKey, []byte("aim/ratchet/chain/a2b/v1"))
	b2a := kdf32(rootKey, []byte("aim/ratchet/chain/b2a/v1"))
	idA, _ := normalizeIDs(localID, contactID)
	if localID == idA {
		return a2b, b2a
	}
	return b2a, a2b
}

func deriveMessageKey(chainKey []byte, idx uint64) ([]byte, []byte) {
	seed := appendUint64Suffix(chainKey, idx)
	msgKey := kdf32(seed, []byte("aim/ratchet/message-key/v1"))
	nextCK := kdf32(seed, []byte("aim/ratchet/chain-key/v1"))
	return msgKey, nextCK
}

func deriveRatchetPubKey(rootKey []byte, idx uint64) []byte {
	seed := appendUint64Suffix(rootKey, idx)
	return kdf32(seed, []byte("aim/ratchet/pubkey/v1"))
}

func envelopeAAD(sessionID, messageID string, chainIndex uint64) []byte {
	b := make([]byte, 0, len(sessionID)+len(messageID)+16)
	b = append(b, []byte(sessionID)...)
	b = append(b, 0)
	b = append(b, []byte(messageID)...)
	b = append(b, 0)
	b = append(b, byte(chainIndex>>56), byte(chainIndex>>48), byte(chainIndex>>40), byte(chainIndex>>32), byte(chainIndex>>24), byte(chainIndex>>16), byte(chainIndex>>8), byte(chainIndex))
	return b
}

func kdf32(input, info []byte) []byte {
	reader := hkdf.New(sha256.New, input, nil, info)
	out := make([]byte, 32)
	_, _ = io.ReadFull(reader, out)
	return out
}

func seen(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func appendSeen(list []string, value string, max int) []string {
	list = append(list, value)
	if len(list) <= max {
		return list
	}
	return append([]string(nil), list[len(list)-max:]...)
}

func pruneSkippedKeys(keys map[uint64][]byte, recvChainIndex uint64, max int) {
	if len(keys) == 0 {
		return
	}
	for idx := range keys {
		// Keep skipped keys in a bounded replay window.
		if idx+maxSkippedChainGap < recvChainIndex {
			delete(keys, idx)
		}
	}
	for len(keys) > max {
		var minIdx uint64
		first := true
		for idx := range keys {
			if first || idx < minIdx {
				minIdx = idx
				first = false
			}
		}
		if first {
			return
		}
		delete(keys, minIdx)
	}
}

func normalizeIDs(a, b string) (string, string) {
	if strings.Compare(a, b) <= 0 {
		return a, b
	}
	return b, a
}

type dhPair struct {
	priv []byte
	pub  []byte
}

func baseX3DHMaterial(required [][]byte, a, b, c dhPair) ([]byte, error) {
	for _, key := range required {
		if len(key) != 32 {
			return nil, ErrInvalidPeerKey
		}
	}
	return combineDHTriplet(a, b, c)
}

func combineDHTriplet(a, b, c dhPair) ([]byte, error) {
	dh1, err := curve25519.X25519(a.priv, a.pub)
	if err != nil {
		return nil, err
	}
	dh2, err := curve25519.X25519(b.priv, b.pub)
	if err != nil {
		return nil, err
	}
	dh3, err := curve25519.X25519(c.priv, c.pub)
	if err != nil {
		return nil, err
	}
	material := append(append(append([]byte{}, dh1...), dh2...), dh3...)
	return material, nil
}

func appendUint64Suffix(base []byte, idx uint64) []byte {
	out := append([]byte{}, base...)
	out = append(out, byte(idx>>56), byte(idx>>48), byte(idx>>40), byte(idx>>32), byte(idx>>24), byte(idx>>16), byte(idx>>8), byte(idx))
	return out
}
