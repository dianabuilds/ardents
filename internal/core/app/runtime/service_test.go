package runtime

import (
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/serviceannounce"
	"github.com/dianabuilds/ardents/internal/core/app/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestServiceAnnounceUpdatesRegistry(t *testing.T) {
	rt := newTestRuntime(t)

	endpoints := []servicedesc.Endpoint{
		{PeerID: "", Addrs: []string{"quic://127.0.0.1:1234"}, Priority: 50},
	}
	node, nodeID, err := servicedesc.BuildDescriptorNode(rt.identity.ID, rt.identity.PrivateKey, "node.fetch.v1", endpoints, nil, map[string]uint64{"max_concurrency": 1})
	if err != nil {
		t.Fatal(err)
	}
	nodeBytes, _, err := contentnode.EncodeWithCID(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.store.Put(nodeID, nodeBytes); err != nil {
		t.Fatal(err)
	}
	body, err := servicedesc.DecodeBody(node)
	if err != nil {
		t.Fatal(err)
	}
	ann := serviceannounce.Announce{
		V:                1,
		ServiceID:        body.ServiceID,
		DescriptorNodeID: nodeID,
		TSMs:             timeutil.NowUnixMs(),
		TTLMs:            int64((2 * time.Minute) / time.Millisecond),
	}
	payload, err := serviceannounce.Encode(ann)
	if err != nil {
		t.Fatal(err)
	}
	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  serviceannounce.Type,
		From: envelope.From{
			PeerID:     rt.peerID,
			IdentityID: rt.identity.ID,
		},
		To: envelope.To{
			PeerID: rt.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if err := env.Sign(rt.identity.PrivateKey); err != nil {
		t.Fatal(err)
	}
	data, err := env.Encode()
	if err != nil {
		t.Fatal(err)
	}
	resps, err := rt.handleEnvelope("peer_x", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps) == 0 {
		t.Fatal("expected ack")
	}
	ackEnv, err := envelope.DecodeEnvelope(resps[0])
	if err != nil {
		t.Fatal(err)
	}
	ap, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if ap.Status != "OK" {
		t.Fatalf("unexpected ack: %s %s", ap.Status, ap.ErrorCode)
	}
	desc, ok := rt.services.Get(body.ServiceID)
	if !ok {
		t.Fatal("descriptor not stored")
	}
	if desc.NodeID != nodeID {
		t.Fatalf("unexpected node id")
	}
}
