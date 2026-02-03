package envelopev2

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func TestEnvelopeV2SignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	identityID, err := ids.NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	serviceID, err := ids.NewServiceID(identityID, "demo.msg.v1")
	if err != nil {
		t.Fatal(err)
	}
	msgID, _ := uuidv7.New()
	env := Envelope{
		V:     Version,
		MsgID: msgID,
		Type:  "demo.msg.v1",
		From:  From{IdentityID: identityID, ServiceID: serviceID},
		To:    To{ServiceID: serviceID},
		TSMs:  time.Now().UTC().UnixNano() / int64(time.Millisecond),
		TTLMs: int64((1 * time.Minute) / time.Millisecond),
	}
	if err := env.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := env.VerifySignature(identityID); err != nil {
		t.Fatal(err)
	}
}

func TestEnvelopeV2ValidateBasic(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	identityID, _ := ids.NewIdentityID(pub)
	serviceID, _ := ids.NewServiceID(identityID, "demo.msg.v1")
	msgID, _ := uuidv7.New()
	env := Envelope{
		V:     Version,
		MsgID: msgID,
		Type:  "demo.msg.v1",
		From:  From{IdentityID: identityID},
		To:    To{ServiceID: serviceID},
		TSMs:  time.Now().UTC().UnixNano() / int64(time.Millisecond),
		TTLMs: int64((1 * time.Minute) / time.Millisecond),
	}
	if err := env.ValidateBasic(time.Now().UTC().UnixNano() / int64(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := env.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := env.VerifySignature(identityID); err != nil {
		t.Fatal(err)
	}
}

func TestEnvelopeV2RejectServiceWithoutIdentity(t *testing.T) {
	msgID, _ := uuidv7.New()
	env := Envelope{
		V:     Version,
		MsgID: msgID,
		Type:  "demo.msg.v1",
		From:  From{ServiceID: "svc_zzz"},
		To:    To{ServiceID: "svc_zzz"},
		TSMs:  time.Now().UTC().UnixNano() / int64(time.Millisecond),
		TTLMs: int64((1 * time.Minute) / time.Millisecond),
	}
	if err := env.ValidateBasic(time.Now().UTC().UnixNano() / int64(time.Millisecond)); err == nil {
		t.Fatal("expected error")
	}
}
