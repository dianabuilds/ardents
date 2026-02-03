package observability

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/perm"
)

type PcapWriter struct {
	mu      sync.Mutex
	enabled bool
	path    string
	file    *os.File
}

type pcapRecord struct {
	Dir             string `json:"dir"`
	TSMs            int64  `json:"ts_ms"`
	PeerIDRemote    string `json:"peer_id_remote,omitempty"`
	EnvelopeCBORB64 string `json:"envelope_cbor_b64"`
}

func NewPcapWriter(enabled bool, path string) *PcapWriter {
	return &PcapWriter{
		enabled: enabled,
		path:    path,
	}
}

func (p *PcapWriter) Enabled() bool {
	if p == nil {
		return false
	}
	return p.enabled
}

func (p *PcapWriter) Write(dir string, peerID string, data []byte) {
	if p == nil || !p.enabled || len(data) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.file == nil {
		f, err := perm.OpenOwnerOnly(p.path)
		if err != nil {
			return
		}
		p.file = f
	}
	rec := pcapRecord{
		Dir:             dir,
		TSMs:            time.Now().UTC().UnixNano() / int64(time.Millisecond),
		PeerIDRemote:    peerID,
		EnvelopeCBORB64: base64.StdEncoding.EncodeToString(data),
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = p.file.Write(append(b, '\n'))
}
