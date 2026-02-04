package runtime

import (
	"bytes"
	"testing"

	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
)

func TestBuildPaddingData_MatchesBucketAndConsumesOneSeq(t *testing.T) {
	r := &Runtime{}
	path := newTestTunnelPath()

	dataBytes, err := r.buildPaddingData(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := tunnel.DecodeData(dataBytes)
	if err != nil {
		t.Fatal(err)
	}
	ctLen := len(data.CT)
	ok := false
	for _, b := range []int{512, 1024, 2048, 4096, 8192, 16384, 32768} {
		if ctLen == b {
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("unexpected ct len: %d", ctLen)
	}
	for i := range path.hops {
		if path.hops[i].seq != 1 {
			t.Fatalf("unexpected seq[%d]=%d (want 1)", i, path.hops[i].seq)
		}
	}
}

func BenchmarkBuildPaddingData(b *testing.B) {
	r := &Runtime{}
	path := newTestTunnelPath()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.buildPaddingData(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func newTestTunnelPath() *tunnelPath {
	return &tunnelPath{
		direction: "outbound",
		hops: []tunnelHop{
			{peerID: "peer-a", tunnelID: bytes.Repeat([]byte{0x01}, 16), key: bytes.Repeat([]byte{0x11}, 32)},
			{peerID: "peer-b", tunnelID: bytes.Repeat([]byte{0x02}, 16), key: bytes.Repeat([]byte{0x22}, 32)},
			{peerID: "peer-c", tunnelID: bytes.Repeat([]byte{0x03}, 16), key: bytes.Repeat([]byte{0x33}, 32)},
		},
	}
}
