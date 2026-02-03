package contentnode

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/ids"
)

func TestEncodeWithCIDAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	n := Node{
		V:           1,
		Type:        "bundle.addressbook.v1",
		CreatedAtMs: time.Now().UTC().UnixNano() / int64(time.Millisecond),
		Owner:       "",
		Links:       []Link{},
		Body:        map[string]any{"entries": []any{}},
		Policy:      map[string]any{"v": uint64(1), "visibility": "public"},
	}
	id, err := ids.NewIdentityID(pub)
	if err != nil {
		t.Fatal(err)
	}
	n.Owner = id
	if err := Sign(&n, priv); err != nil {
		t.Fatal(err)
	}
	nodeBytes, cidStr, err := EncodeWithCID(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyBytes(nodeBytes, cidStr); err != nil {
		t.Fatal(err)
	}
	_ = pub
}
