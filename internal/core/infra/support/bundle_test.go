package support

import (
	"encoding/json"
	"testing"
)

func TestRedactAddressBookJSON_RemovesNote(t *testing.T) {
	in := []byte(`{"v":1,"entries":[{"alias":"a","target_type":"peer","target_id":"x","note":"secret"}]}`)
	out, err := redactAddressBookJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	entries := obj["entries"].([]any)
	e := entries[0].(map[string]any)
	if _, ok := e["note"]; ok {
		t.Fatal("expected note to be redacted")
	}
}
