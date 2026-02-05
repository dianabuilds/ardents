package support

import (
	"encoding/json"
	"strings"
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

func TestRedactLogTail_RedactsSensitiveKeys(t *testing.T) {
	in := []byte(`{"level":"info","token":"abc","nested":{"secret":"def"}}` + "\n" +
		`{"level":"warn","password":"p@ss","api_key":"k"}` + "\n")
	out := redactLogTail(in)
	if strings.Contains(string(out), "abc") || strings.Contains(string(out), "def") {
		t.Fatal("expected secrets to be redacted")
	}
	if !strings.Contains(string(out), "\"token\":\"[redacted]\"") {
		t.Fatal("expected token to be redacted")
	}
	if !strings.Contains(string(out), "\"secret\":\"[redacted]\"") {
		t.Fatal("expected secret to be redacted")
	}
	if !strings.Contains(string(out), "\"password\":\"[redacted]\"") {
		t.Fatal("expected password to be redacted")
	}
}

func TestRedactConfigJSON_RedactsSensitiveKeys(t *testing.T) {
	in := []byte(`{"v":1,"integration":{"token":"abc"},"auth":{"api_key":"k"}}`)
	out, err := redactConfigJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	integration := obj["integration"].(map[string]any)
	if integration["token"] != "[redacted]" {
		t.Fatal("expected token to be redacted")
	}
	auth := obj["auth"].(map[string]any)
	if auth["api_key"] != "[redacted]" {
		t.Fatal("expected api_key to be redacted")
	}
}
