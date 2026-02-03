package runtime

import (
	"errors"
	"testing"
	"time"

	netdbsvc "github.com/dianabuilds/ardents/internal/core/app/services/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/relay"
	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestRelaySingleHopDelivery(t *testing.T) {
	sender, relays, receiver := setupRelayPeers(t, 1)
	relayRT := relays[0]

	relayRT.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == receiver.peerID {
			_, err := receiver.HandleEnvelope(relayRT.peerID, envBytes)
			return err
		}
		return nil
	})

	wireRelayForwarder(receiver, sender)

	finalBytes := buildFindNodeEnv(t, sender.peerID, receiver.peerID)
	pktBytes := buildRelayPacket(t, receiver.peerID, finalBytes, relayRT.transportKeys.PublicKey)

	ap := sendRelayAndAck(t, sender, relayRT, pktBytes)
	if ap.Status != "OK" {
		t.Fatalf("unexpected ack status: %s", ap.Status)
	}
}

func TestRelayDropReject(t *testing.T) {
	sender, relays, receiver := setupRelayPeers(t, 1)
	relayRT := relays[0]

	relayRT.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		return errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	})

	finalBytes := buildFindNodeEnv(t, sender.peerID, receiver.peerID)
	pktBytes := buildRelayPacket(t, receiver.peerID, finalBytes, relayRT.transportKeys.PublicKey)

	ap := sendRelayAndAck(t, sender, relayRT, pktBytes)
	if ap.Status != "REJECTED" || ap.ErrorCode != "ERR_RELAY_NEXT_HOP_UNREACHABLE" {
		t.Fatalf("unexpected ack: %s %s", ap.Status, ap.ErrorCode)
	}
}

func TestRelayTwoHopDelivery(t *testing.T) {
	sender, relays, receiver := setupRelayPeers(t, 2)
	relayA := relays[0]
	relayB := relays[1]

	relayA.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == relayB.peerID {
			_, err := relayB.HandleEnvelope(relayA.peerID, envBytes)
			return err
		}
		return nil
	})

	relayB.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == receiver.peerID {
			_, err := receiver.HandleEnvelope(relayB.peerID, envBytes)
			return err
		}
		return nil
	})

	wireRelayForwarder(receiver, sender)

	finalBytes := buildFindNodeEnv(t, sender.peerID, receiver.peerID)
	pktBBytes := buildRelayPacket(t, receiver.peerID, finalBytes, relayB.transportKeys.PublicKey)
	pktABytes := buildRelayPacket(t, relayB.peerID, pktBBytes, relayA.transportKeys.PublicKey)

	ap := sendRelayAndAck(t, sender, relayA, pktABytes)
	if ap.Status != "OK" {
		t.Fatalf("unexpected ack status: %s", ap.Status)
	}
}

const relayTTLMS = int64((1 * time.Minute) / time.Millisecond)

func buildFindNodePayload(t *testing.T) []byte {
	t.Helper()
	req := netdbsvc.FindNode{
		V:   netdbsvc.Version,
		Key: make([]byte, 32),
	}
	payload, err := netdbsvc.EncodeFindNode(req)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func buildFindNodeEnv(t *testing.T, fromPeerID string, toPeerID string) []byte {
	t.Helper()
	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	payload := buildFindNodePayload(t)
	finalEnv := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  netdbsvc.FindNodeType,
		From: envelope.From{
			PeerID: fromPeerID,
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   relayTTLMS,
		Payload: payload,
	}
	finalBytes, err := finalEnv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return finalBytes
}

func buildRelayPacket(t *testing.T, toPeerID string, payload []byte, pubKey []byte) []byte {
	t.Helper()
	pkt, err := relay.Build(toPeerID, relayTTLMS, payload, pubKey)
	if err != nil {
		t.Fatal(err)
	}
	pktBytes, err := relay.Encode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	return pktBytes
}

func sendRelayAndAck(t *testing.T, sender *Runtime, relayRT *Runtime, pktBytes []byte) ack.Payload {
	t.Helper()
	relayEnv, err := sender.buildRelayEnvelope(relayRT.peerID, pktBytes)
	if err != nil {
		t.Fatal(err)
	}
	resps, err := relayRT.HandleEnvelope(sender.peerID, relayEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps) == 0 {
		t.Fatal("expected ack from relay")
	}
	return decodeAckPayload(t, resps[0])
}

func decodeAckPayload(t *testing.T, envBytes []byte) ack.Payload {
	t.Helper()
	ackEnv, err := envelope.DecodeEnvelope(envBytes)
	if err != nil {
		t.Fatal(err)
	}
	ap, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return ap
}

func setupRelayPeers(t *testing.T, relayCount int) (*Runtime, []*Runtime, *Runtime) {
	t.Helper()
	cfg := config.Default()
	sender := NewSim(cfg, "", testIdentity(t), testBook())
	relays := make([]*Runtime, 0, relayCount)
	for i := 0; i < relayCount; i++ {
		relays = append(relays, NewSim(cfg, "", testIdentity(t), testBook()))
	}
	receiver := NewSim(cfg, "", testIdentity(t), testBook())
	for _, r := range relays {
		trustIdentity(r, sender.identity.ID)
	}
	trustIdentity(receiver, sender.identity.ID)
	return sender, relays, receiver
}

func wireRelayForwarder(from *Runtime, to *Runtime) {
	from.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == to.peerID {
			_, err := to.HandleEnvelope(from.peerID, envBytes)
			return err
		}
		return nil
	})
}

func testIdentity(t *testing.T) identity.Identity {
	id, err := identity.NewEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testBook() addressbook.Book {
	return addressbook.Book{V: 1, Entries: []addressbook.Entry{}}
}

func trustIdentity(rt *Runtime, id string) {
	rt.book.Entries = append(rt.book.Entries, addressbook.Entry{
		Alias:       "trusted",
		TargetType:  "identity",
		TargetID:    id,
		Source:      "self",
		Trust:       "trusted",
		CreatedAtMs: timeutil.NowUnixMs(),
	})
}
