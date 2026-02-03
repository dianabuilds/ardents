package routerinfo

import "github.com/dianabuilds/ardents/internal/shared/codec"

func UnsignedBytes(
	v uint64,
	peerID string,
	transportPub []byte,
	onionPub []byte,
	addrs []string,
	relay bool,
	netdb bool,
	publishedAtMs int64,
	expiresAtMs int64,
) ([]byte, error) {
	type caps struct {
		Relay bool `cbor:"relay"`
		NetDB bool `cbor:"netdb"`
	}
	type unsigned struct {
		V             uint64   `cbor:"v"`
		PeerID        string   `cbor:"peer_id"`
		TransportPub  []byte   `cbor:"transport_pub"`
		OnionPub      []byte   `cbor:"onion_pub"`
		Addrs         []string `cbor:"addrs"`
		Caps          caps     `cbor:"caps"`
		PublishedAtMs int64    `cbor:"published_at_ms"`
		ExpiresAtMs   int64    `cbor:"expires_at_ms"`
	}
	u := unsigned{
		V:             v,
		PeerID:        peerID,
		TransportPub:  transportPub,
		OnionPub:      onionPub,
		Addrs:         addrs,
		Caps:          caps{Relay: relay, NetDB: netdb},
		PublishedAtMs: publishedAtMs,
		ExpiresAtMs:   expiresAtMs,
	}
	return codec.Marshal(u)
}
