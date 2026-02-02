package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/relay"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestRelaySingleHopDelivery(t *testing.T) {
	cfg := config.Default()
	sender := NewSim(cfg, "", testIdentity(t), testBook())
	relayRT := NewSim(cfg, "", testIdentity(t), testBook())
	receiver := NewSim(cfg, "", testIdentity(t), testBook())
	trustIdentity(relayRT, sender.identity.ID)
	trustIdentity(receiver, sender.identity.ID)

	relayRT.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == receiver.peerID {
			_, err := receiver.HandleEnvelope(relayRT.peerID, envBytes)
			return err
		}
		return nil
	})

	receiver.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == sender.peerID {
			_, err := sender.HandleEnvelope(receiver.peerID, envBytes)
			return err
		}
		return nil
	})

	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	finalEnv := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  "chat.msg.v1",
		From: envelope.From{
			PeerID: sender.peerID,
		},
		To: envelope.To{
			PeerID: receiver.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: []byte{0x01},
	}
	finalBytes, err := finalEnv.Encode()
	if err != nil {
		t.Fatal(err)
	}

	pkt, err := relay.Build(receiver.peerID, int64((1*time.Minute)/time.Millisecond), finalBytes, relayRT.transportKeys.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pktBytes, err := relay.Encode(pkt)
	if err != nil {
		t.Fatal(err)
	}

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
	ackEnv, err := envelope.DecodeEnvelope(resps[0])
	if err != nil {
		t.Fatal(err)
	}
	ap, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if ap.Status != "OK" {
		t.Fatalf("unexpected ack status: %s", ap.Status)
	}
}

func TestRelayDropReject(t *testing.T) {
	cfg := config.Default()
	sender := NewSim(cfg, "", testIdentity(t), testBook())
	relayRT := NewSim(cfg, "", testIdentity(t), testBook())
	receiver := NewSim(cfg, "", testIdentity(t), testBook())
	trustIdentity(relayRT, sender.identity.ID)
	trustIdentity(receiver, sender.identity.ID)

	relayRT.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		return errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	})

	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	finalEnv := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  "chat.msg.v1",
		From: envelope.From{
			PeerID: sender.peerID,
		},
		To: envelope.To{
			PeerID: receiver.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: []byte{0x01},
	}
	finalBytes, err := finalEnv.Encode()
	if err != nil {
		t.Fatal(err)
	}

	pkt, err := relay.Build(receiver.peerID, int64((1*time.Minute)/time.Millisecond), finalBytes, relayRT.transportKeys.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pktBytes, err := relay.Encode(pkt)
	if err != nil {
		t.Fatal(err)
	}

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
	ackEnv, err := envelope.DecodeEnvelope(resps[0])
	if err != nil {
		t.Fatal(err)
	}
	ap, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if ap.Status != "REJECTED" || ap.ErrorCode != "ERR_RELAY_NEXT_HOP_UNREACHABLE" {
		t.Fatalf("unexpected ack: %s %s", ap.Status, ap.ErrorCode)
	}
}

func TestRelayTwoHopDelivery(t *testing.T) {
	cfg := config.Default()
	sender := NewSim(cfg, "", testIdentity(t), testBook())
	relayA := NewSim(cfg, "", testIdentity(t), testBook())
	relayB := NewSim(cfg, "", testIdentity(t), testBook())
	receiver := NewSim(cfg, "", testIdentity(t), testBook())
	trustIdentity(relayA, sender.identity.ID)
	trustIdentity(relayB, sender.identity.ID)
	trustIdentity(receiver, sender.identity.ID)

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

	receiver.SetRelayForwarder(func(peerID string, envBytes []byte) error {
		if peerID == sender.peerID {
			_, err := sender.HandleEnvelope(receiver.peerID, envBytes)
			return err
		}
		return nil
	})

	msgID, err := uuidv7.New()
	if err != nil {
		t.Fatal(err)
	}
	finalEnv := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  "chat.msg.v1",
		From: envelope.From{
			PeerID: sender.peerID,
		},
		To: envelope.To{
			PeerID: receiver.peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: []byte{0x01},
	}
	finalBytes, err := finalEnv.Encode()
	if err != nil {
		t.Fatal(err)
	}

	pktB, err := relay.Build(receiver.peerID, int64((1*time.Minute)/time.Millisecond), finalBytes, relayB.transportKeys.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pktBBytes, err := relay.Encode(pktB)
	if err != nil {
		t.Fatal(err)
	}
	pktA, err := relay.Build(relayB.peerID, int64((1*time.Minute)/time.Millisecond), pktBBytes, relayA.transportKeys.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pktABytes, err := relay.Encode(pktA)
	if err != nil {
		t.Fatal(err)
	}

	relayEnv, err := sender.buildRelayEnvelope(relayA.peerID, pktABytes)
	if err != nil {
		t.Fatal(err)
	}
	resps, err := relayA.HandleEnvelope(sender.peerID, relayEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(resps) == 0 {
		t.Fatal("expected ack from relayA")
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
		t.Fatalf("unexpected ack status: %s", ap.Status)
	}
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
