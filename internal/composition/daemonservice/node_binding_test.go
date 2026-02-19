package daemonservice

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"aim-chat/go-backend/internal/waku"
)

func newNodeBindingTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("AIM_STORAGE_PASSPHRASE", "binding-test-secret")
	cfg := waku.DefaultConfig()
	cfg.Transport = waku.TransportMock
	svc, err := NewServiceForDaemonWithDataDir(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, _, err := svc.CreateIdentity("pass"); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	return svc
}

func TestNodeBindingBindAndUnbindFlow(t *testing.T) {
	svc := newNodeBindingTestService(t)
	var err error

	link, err := svc.CreateNodeBindingLinkCode(120)
	if err != nil {
		t.Fatalf("create link code: %v", err)
	}

	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}
	nodeID := "node-1"
	challengePayload := nodeBindingChallengeBytes(link.IdentityID, link.LinkCode, nodeID, link.Challenge)
	nodeSig := ed25519.Sign(nodePriv, challengePayload)

	record, err := svc.CompleteNodeBinding(
		link.LinkCode,
		nodeID,
		base64.StdEncoding.EncodeToString(nodePub),
		base64.StdEncoding.EncodeToString(nodeSig),
		false,
	)
	if err != nil {
		t.Fatalf("complete binding: %v", err)
	}
	if record.NodeID != nodeID || record.IdentityID == "" {
		t.Fatalf("unexpected binding record: %+v", record)
	}

	stored, exists, err := svc.GetNodeBinding()
	if err != nil {
		t.Fatalf("get binding: %v", err)
	}
	if !exists || stored.NodeID != nodeID {
		t.Fatalf("unexpected stored binding: exists=%v record=%+v", exists, stored)
	}

	rebindLink, err := svc.CreateNodeBindingLinkCode(120)
	if err != nil {
		t.Fatalf("create rebind link code: %v", err)
	}
	secondNodePub, secondNodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate second node key: %v", err)
	}
	secondNodeID := "node-2"
	rebindPayload := nodeBindingChallengeBytes(rebindLink.IdentityID, rebindLink.LinkCode, secondNodeID, rebindLink.Challenge)
	rebindSig := ed25519.Sign(secondNodePriv, rebindPayload)
	if _, err := svc.CompleteNodeBinding(
		rebindLink.LinkCode,
		secondNodeID,
		base64.StdEncoding.EncodeToString(secondNodePub),
		base64.StdEncoding.EncodeToString(rebindSig),
		false,
	); err == nil {
		t.Fatal("expected rebind without confirmation to be rejected")
	}
	rebindLink2, err := svc.CreateNodeBindingLinkCode(120)
	if err != nil {
		t.Fatalf("create rebind link code 2: %v", err)
	}
	rebindPayload2 := nodeBindingChallengeBytes(rebindLink2.IdentityID, rebindLink2.LinkCode, secondNodeID, rebindLink2.Challenge)
	rebindSig2 := ed25519.Sign(secondNodePriv, rebindPayload2)
	if _, err := svc.CompleteNodeBinding(
		rebindLink2.LinkCode,
		secondNodeID,
		base64.StdEncoding.EncodeToString(secondNodePub),
		base64.StdEncoding.EncodeToString(rebindSig2),
		true,
	); err != nil {
		t.Fatalf("expected confirmed rebind to succeed, got: %v", err)
	}

	removed, err := svc.UnbindNode(secondNodeID, true)
	if err != nil {
		t.Fatalf("unbind node: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}
	_, exists, err = svc.GetNodeBinding()
	if err != nil {
		t.Fatalf("get binding after unbind: %v", err)
	}
	if exists {
		t.Fatal("expected no binding after unbind")
	}
}

func TestNodeBindingRejectsInvalidOrExpiredLinkCode(t *testing.T) {
	svc := newNodeBindingTestService(t)
	var err error

	link, err := svc.CreateNodeBindingLinkCode(120)
	if err != nil {
		t.Fatalf("create link code: %v", err)
	}
	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}
	nodeID := "node-1"
	invalidSig := ed25519.Sign(nodePriv, []byte("not-the-right-payload"))
	if _, err := svc.CompleteNodeBinding(
		link.LinkCode,
		nodeID,
		base64.StdEncoding.EncodeToString(nodePub),
		base64.StdEncoding.EncodeToString(invalidSig),
		false,
	); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}

	expiredLink, err := svc.CreateNodeBindingLinkCode(1)
	if err != nil {
		t.Fatalf("create short ttl link code: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	validPayload := nodeBindingChallengeBytes(expiredLink.IdentityID, expiredLink.LinkCode, nodeID, expiredLink.Challenge)
	validSig := ed25519.Sign(nodePriv, validPayload)
	if _, err := svc.CompleteNodeBinding(
		expiredLink.LinkCode,
		nodeID,
		base64.StdEncoding.EncodeToString(nodePub),
		base64.StdEncoding.EncodeToString(validSig),
		false,
	); err == nil {
		t.Fatal("expected expired link code to be rejected")
	}
}
