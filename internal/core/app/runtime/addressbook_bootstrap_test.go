package runtime

import (
	"testing"

	"github.com/dianabuilds/ardents/internal/core/infra/addressbook"
	"github.com/dianabuilds/ardents/internal/shared/testutil"
)

func TestAddressBookBootstrapPeers(t *testing.T) {
	_, _, peerID1 := testutil.NewPeerKeyAndID(t)
	_, _, peerID2 := testutil.NewPeerKeyAndID(t)
	_, _, peerID3 := testutil.NewPeerKeyAndID(t)

	now := int64(1000)
	book := addressbook.Book{
		V: 1,
		Entries: []addressbook.Entry{
			{
				Alias:       "p1",
				TargetType:  "peer",
				TargetID:    peerID1,
				Trust:       "trusted",
				Source:      "self",
				CreatedAtMs: 1,
				Note:        "quic://127.0.0.1:1111",
			},
			{
				Alias:       "p2",
				TargetType:  "peer",
				TargetID:    peerID2,
				Trust:       "trusted",
				Source:      "self",
				CreatedAtMs: 2,
				Note:        "127.0.0.1:2222, quic://127.0.0.1:3333",
			},
			{
				Alias:       "p3",
				TargetType:  "peer",
				TargetID:    peerID3,
				Trust:       "trusted",
				Source:      "self",
				CreatedAtMs: 3,
				ExpiresAtMs: 10,
				Note:        "127.0.0.1:4444",
			},
			{
				Alias:       "u1",
				TargetType:  "peer",
				TargetID:    "peer_invalid",
				Trust:       "trusted",
				Source:      "self",
				CreatedAtMs: 4,
				Note:        "127.0.0.1:5555",
			},
			{
				Alias:       "u2",
				TargetType:  "peer",
				TargetID:    peerID1,
				Trust:       "untrusted",
				Source:      "self",
				CreatedAtMs: 5,
				Note:        "127.0.0.1:6666",
			},
		},
	}
	rt := Runtime{book: book}
	peers := rt.addressBookBootstrapPeers(now)

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
	if peers[0].PeerID == peerID3 || peers[1].PeerID == peerID3 {
		t.Fatal("expired peer should be ignored")
	}
}
