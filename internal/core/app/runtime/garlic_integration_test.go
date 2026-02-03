package runtime

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/garlic"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/lockeys"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestGarlicEnvelopeV2TaskFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv(appdirs.EnvHome, home)

	cfg := config.Default()
	rt := New(cfg)

	localServiceID := mustServiceID(t, rt.identity.ID, "chat.msg.v1")
	dirs, err := appdirs.Resolve(home)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lockeys.LoadOrCreate(dirs.LKeysDir(), localServiceID)
	if err != nil {
		t.Fatal(err)
	}
	localKey, err := lockeys.Load(dirs.LKeysDir(), localServiceID)
	if err != nil {
		t.Fatal(err)
	}

	replyServiceName := "mailbox.msg.v1"
	replyOwnerPub, replyOwnerPriv, _ := ed25519.GenerateKey(nil)
	replyOwnerID, _ := ids.NewIdentityID(replyOwnerPub)
	replyServiceID, _ := ids.NewServiceID(replyOwnerID, replyServiceName)
	replyEncPriv := make([]byte, 32)
	if _, err := rand.Read(replyEncPriv); err != nil {
		t.Fatal(err)
	}
	replyEncPub, _ := curve25519.X25519(replyEncPriv, curve25519.Basepoint)
	gatewayPeerID := mustPeerID(t)
	leaseTunnelID := make([]byte, 16)
	if _, err := rand.Read(leaseTunnelID); err != nil {
		t.Fatal(err)
	}
	nowMs := timeutil.NowUnixMs()
	leaseSet := netdb.LeaseSet{
		V:               1,
		ServiceID:       replyServiceID,
		OwnerIdentityID: replyOwnerID,
		ServiceName:     replyServiceName,
		EncPub:          replyEncPub,
		Leases: []netdb.Lease{
			{GatewayPeerID: gatewayPeerID, TunnelID: leaseTunnelID, ExpiresAtMs: nowMs + 60_000},
		},
		PublishedAtMs: nowMs,
		ExpiresAtMs:   nowMs + 60_000,
	}
	leaseSet, err = netdb.SignLeaseSet(replyOwnerPriv, leaseSet)
	if err != nil {
		t.Fatal(err)
	}
	leaseBytes, err := netdb.EncodeLeaseSet(leaseSet)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := rt.netdb.Store(leaseBytes, nowMs); status != "OK" {
		t.Fatal("failed to store lease set")
	}

	outHopKey := make([]byte, 32)
	if _, err := rand.Read(outHopKey); err != nil {
		t.Fatal(err)
	}
	outHopID := make([]byte, 16)
	if _, err := rand.Read(outHopID); err != nil {
		t.Fatal(err)
	}
	rt.tunnelMgrMu.Lock()
	rt.outboundTunnels = []*tunnelPath{
		{
			direction:   "outbound",
			createdAtMs: nowMs,
			expiresAtMs: nowMs + 60_000,
			hops: []tunnelHop{
				{peerID: gatewayPeerID, tunnelID: outHopID, key: outHopKey},
			},
		},
	}
	rt.tunnelMgrMu.Unlock()

	var captured []byte
	rt.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID != gatewayPeerID {
			t.Fatalf("unexpected peer %s", peerID)
		}
		captured = envBytes
		return nil
	})

	taskReq := tasks.Request{
		V:               tasks.Version,
		TaskID:          mustUUIDv2(t),
		ClientRequestID: mustUUIDv2(t),
		JobType:         "ai.chat.v1",
		Input:           map[string]any{"v": uint64(1)},
		TSMs:            nowMs,
	}
	reqBytes, _ := tasks.EncodeRequest(taskReq)
	env := envelopev2.Envelope{
		V:     envelopev2.Version,
		MsgID: mustUUIDv2(t),
		Type:  tasks.RequestType,
		From:  envelopev2.From{IdentityID: rt.identity.ID},
		To:    envelopev2.To{ServiceID: localServiceID},
		ReplyTo: &envelopev2.Reply{
			ServiceID: replyServiceID,
		},
		TSMs:    nowMs,
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: reqBytes,
	}
	if err := env.Sign(rt.identity.PrivateKey); err != nil {
		t.Fatal(err)
	}
	envBytes, _ := env.Encode()

	inner := garlic.Inner{
		V:           garlic.Version,
		ExpiresAtMs: nowMs + 60_000,
		Cloves: []garlic.Clove{
			{Kind: "envelope", Envelope: envBytes},
		},
	}
	msg, err := garlic.Encrypt(localServiceID, localKey.Public, inner)
	if err != nil {
		t.Fatal(err)
	}
	msgBytes, _ := garlic.Encode(msg)
	if err := rt.handleGarlic(msgBytes); err != nil {
		t.Fatal(err)
	}

	if len(captured) == 0 {
		t.Fatal("expected outbound tunnel delivery")
	}
	outer, err := envelope.DecodeEnvelope(captured)
	if err != nil || outer.Type != tunnel.DataType {
		t.Fatal("expected tunnel.data.v1 envelope")
	}
	data, err := tunnel.DecodeData(outer.Payload)
	if err != nil {
		t.Fatal(err)
	}
	innerData, err := tunnel.DecryptData(outHopKey, data.CT)
	if err != nil || innerData.Kind != "deliver" {
		t.Fatal("expected deliver inner")
	}
	garlicMsg, err := garlic.Decode(innerData.Garlic)
	if err != nil {
		t.Fatal(err)
	}
	replyInner, err := garlic.Decrypt(garlicMsg, replyEncPriv)
	if err != nil {
		t.Fatal(err)
	}
	if len(replyInner.Cloves) == 0 {
		t.Fatal("expected reply cloves")
	}
	replyEnv, err := envelopev2.DecodeEnvelope(replyInner.Cloves[0].Envelope)
	if err != nil {
		t.Fatal(err)
	}
	if replyEnv.Type != tasks.AcceptType && replyEnv.Type != tasks.ResultType && replyEnv.Type != tasks.FailType {
		t.Fatalf("unexpected reply type: %s", replyEnv.Type)
	}
}

func mustServiceID(t *testing.T, identityID, serviceName string) string {
	t.Helper()
	id, err := ids.NewServiceID(identityID, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustPeerID(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	peerID, err := ids.NewPeerID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return peerID
}

func mustUUIDv2(t *testing.T) string {
	t.Helper()
	id, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
