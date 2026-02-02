package pow

import "testing"

func TestPowGenerateVerify(t *testing.T) {
	sub := Subject("msg", 1, "peer_x")
	stamp, err := Generate(sub, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(stamp); err != nil {
		t.Fatal(err)
	}
}
