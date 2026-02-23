package nodeagent

import (
	"aim-chat/go-backend/internal/bootstrap/enrollmenttoken"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stateFileName          = "node-agent-state.json"
	redeemedStoreFileName  = "enrollment-redeemed.json"
	defaultRPCProbeTimeout = 2 * time.Second
	defaultNodeProfileID   = "network_assist_default"
)

type EnrollmentState struct {
	TokenID          string    `json:"token_id"`
	Issuer           string    `json:"issuer"`
	Scope            string    `json:"scope"`
	SubjectNodeGroup string    `json:"subject_node_group"`
	KeyID            string    `json:"key_id"`
	ExpiresAt        time.Time `json:"expires_at"`
	EnrolledAt       time.Time `json:"enrolled_at"`
}

type State struct {
	SchemaVersion        int              `json:"schema_version"`
	NodeID               string           `json:"node_id"`
	NodePublicKeyBase64  string           `json:"node_public_key_base64"`
	NodePrivateKeyBase64 string           `json:"node_private_key_base64"`
	InitializedAt        time.Time        `json:"initialized_at"`
	Enrollment           *EnrollmentState `json:"enrollment,omitempty"`
}

type Status struct {
	NodeID      string    `json:"node_id"`
	Initialized bool      `json:"initialized"`
	Enrolled    bool      `json:"enrolled"`
	ProfileID   string    `json:"profile_id,omitempty"`
	Health      string    `json:"health"`
	PeerCount   int       `json:"peer_count"`
	CheckedAt   time.Time `json:"checked_at"`
	Source      string    `json:"source"`
	LastError   string    `json:"last_error,omitempty"`
}

type Service struct {
	dataDir string
	now     func() time.Time
	probe   func(ctx context.Context, rpcAddr, rpcToken string) (int, error)
}

func New(dataDir string) *Service {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "."
	}
	return &Service{
		dataDir: dataDir,
		now:     func() time.Time { return time.Now().UTC() },
		probe:   probePeerCount,
	}
}

func (s *Service) Init() (State, bool, error) {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return State{}, false, err
	}
	existing, exists, err := s.loadState()
	if err != nil {
		return State{}, false, err
	}
	if exists {
		return existing, false, nil
	}

	pub, prv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return State{}, false, err
	}
	state := State{
		SchemaVersion:        1,
		NodeID:               generateNodeID(pub),
		NodePublicKeyBase64:  base64.StdEncoding.EncodeToString(pub),
		NodePrivateKeyBase64: base64.StdEncoding.EncodeToString(prv),
		InitializedAt:        s.now(),
	}
	if err := s.saveState(state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (s *Service) Enroll(token string, issuerKeys map[string]ed25519.PublicKey) (EnrollmentState, error) {
	state, exists, err := s.loadState()
	if err != nil {
		return EnrollmentState{}, err
	}
	if !exists {
		return EnrollmentState{}, errors.New("node-agent is not initialized")
	}
	store := enrollmenttoken.NewFileStore(filepath.Join(s.dataDir, redeemedStoreFileName))
	if err := store.Bootstrap(); err != nil {
		return EnrollmentState{}, err
	}
	verifier := enrollmenttoken.Verifier{
		RequiredIssuer: enrollmenttoken.RequiredIssuer,
		RequiredScope:  enrollmenttoken.RequiredScope,
		PublicKeys:     issuerKeys,
		Now:            s.now,
	}
	claims, _, err := verifier.VerifyAndRedeem(token, store)
	if err != nil {
		return EnrollmentState{}, err
	}
	enrollment := &EnrollmentState{
		TokenID:          claims.TokenID,
		Issuer:           claims.Issuer,
		Scope:            claims.Scope,
		SubjectNodeGroup: claims.SubjectNodeGroup,
		KeyID:            claims.KeyID,
		ExpiresAt:        claims.ExpiresAt.UTC(),
		EnrolledAt:       s.now(),
	}
	state.Enrollment = enrollment
	if err := s.saveState(state); err != nil {
		return EnrollmentState{}, err
	}
	return *enrollment, nil
}

func (s *Service) Status(ctx context.Context, rpcAddr, rpcToken string) (Status, error) {
	state, exists, err := s.loadState()
	if err != nil {
		return Status{}, err
	}
	now := s.now()
	if !exists {
		return Status{
			Initialized: false,
			Enrolled:    false,
			ProfileID:   defaultNodeProfileID,
			Health:      "uninitialized",
			PeerCount:   0,
			CheckedAt:   now,
			Source:      "local",
		}, nil
	}
	status := Status{
		NodeID:      state.NodeID,
		Initialized: true,
		Enrolled:    state.Enrollment != nil,
		ProfileID:   defaultNodeProfileID,
		Health:      "initialized",
		PeerCount:   0,
		CheckedAt:   now,
		Source:      "local",
	}
	if state.Enrollment != nil {
		if state.Enrollment.ExpiresAt.After(now) {
			status.Health = "enrolled"
		} else {
			status.Health = "degraded"
			status.LastError = "enrollment token has expired"
		}
	}
	if strings.TrimSpace(rpcAddr) != "" {
		peerCount, err := s.probe(ctx, rpcAddr, rpcToken)
		if err == nil {
			status.PeerCount = peerCount
			status.Source = "rpc"
		} else if status.LastError == "" {
			status.LastError = err.Error()
		}
	}
	return status, nil
}

func (s *Service) loadState() (State, bool, error) {
	path := filepath.Join(s.dataDir, stateFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, false, nil
		}
		return State{}, false, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (s *Service) saveState(state State) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dataDir, stateFileName)
	return os.WriteFile(path, raw, 0o600)
}

func generateNodeID(publicKey ed25519.PublicKey) string {
	encoded := base64.RawURLEncoding.EncodeToString(publicKey)
	if len(encoded) > 24 {
		encoded = encoded[:24]
	}
	return "node_" + encoded
}

func probePeerCount(ctx context.Context, rpcAddr, rpcToken string) (peerCount int, retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, defaultRPCProbeTimeout)
	defer cancel()

	body := `{"jsonrpc":"2.0","id":1,"method":"network.status","params":[]}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+strings.TrimSpace(rpcAddr), strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(rpcToken) != "" {
		req.Header.Set("X-AIM-RPC-Token", strings.TrimSpace(rpcToken))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("rpc status %d", resp.StatusCode)
	}
	var decoded struct {
		Result struct {
			PeerCount int `json:"peer_count"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return 0, err
	}
	if decoded.Error != nil {
		return 0, errors.New("rpc returned error")
	}
	return decoded.Result.PeerCount, nil
}
