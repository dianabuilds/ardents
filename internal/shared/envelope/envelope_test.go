package envelope

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func TestEnvelopeSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityID, err := ids.NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	env := Envelope{
		V:       Version,
		MsgID:   "01890b86-6f97-7c92-9b87-7b2d0d72331b",
		Type:    "demo.msg.v1",
		From:    From{PeerID: "peer_bafkrw2xqkvpw3ehny6qo2rwcynjb36zl24d3p5y3g6g6qg4keq3f"},
		To:      To{PeerID: "peer_bafkrw2xqkvpw3ehny6qo2rwcynjb36zl24d3p5y3g6g6qg4keq3g"},
		TSMs:    1,
		TTLMs:   1000,
		Payload: []byte{0x01, 0x02},
	}
	if err := env.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := env.VerifySignature(identityID); err != nil {
		t.Fatal(err)
	}
}
