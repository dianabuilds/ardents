package nodefetch

import "testing"

func TestEncodeDecodeRequest(t *testing.T) {
	req := Request{V: Version, NodeID: "cidv1-example"}
	data, err := EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.V != req.V || out.NodeID != req.NodeID {
		t.Fatalf("unexpected request decode: %+v", out)
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	resp := Response{V: Version, NodeBytes: []byte{0x01, 0x02}}
	data, err := EncodeResponse(resp)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if out.V != resp.V || string(out.NodeBytes) != string(resp.NodeBytes) {
		t.Fatalf("unexpected response decode: %+v", out)
	}
}
