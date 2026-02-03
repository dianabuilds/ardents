package runtime

import (
	"encoding/hex"
	"errors"

	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

var (
	ErrTunnelRejected = errors.New("ERR_TUNNEL_BUILD_REJECTED")
)

type tunnelSession struct {
	key          []byte
	nextPeerID   string
	nextTunnelID []byte
	expiresAtMs  int64
	lastSeq      uint64
}

func (r *Runtime) tunnelKey(id []byte) string {
	return hex.EncodeToString(id)
}

func (r *Runtime) storeTunnel(id []byte, s *tunnelSession) {
	if r == nil || len(id) == 0 {
		return
	}
	r.tunnelMu.Lock()
	defer r.tunnelMu.Unlock()
	if r.tunnels == nil {
		r.tunnels = make(map[string]*tunnelSession)
	}
	r.tunnels[r.tunnelKey(id)] = s
}

func (r *Runtime) loadTunnel(id []byte) (*tunnelSession, bool) {
	r.tunnelMu.Lock()
	defer r.tunnelMu.Unlock()
	if r.tunnels == nil {
		return nil, false
	}
	s, ok := r.tunnels[r.tunnelKey(id)]
	return s, ok
}

func (r *Runtime) handleTunnelBuild(fromPeerID string, payload []byte) ([][]byte, error) {
	req, err := tunnel.DecodeBuildRequest(payload)
	if err != nil {
		reply := tunnel.BuildReply{V: 1, Status: "REJECTED", Error: tunnel.ErrBuildDecode.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	if req.V != 1 || len(req.TunnelID) != 16 {
		reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "REJECTED", Error: tunnel.ErrBuildDecode.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	if len(r.onionKey.Private) != 32 {
		reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "REJECTED", Error: ErrTunnelRejected.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	rec, hopKey, err := tunnel.DecryptRecord(req, r.onionKey.Private, r.peerID)
	if err != nil {
		reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "REJECTED", Error: err.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	if rec.ExpiresAtMs > 0 && timeutil.NowUnixMs() > rec.ExpiresAtMs {
		reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "REJECTED", Error: tunnel.ErrBuildDecode.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	if rec.NextPeerID != "" && len(rec.NextTunnelID) != 16 {
		reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "REJECTED", Error: tunnel.ErrBuildDecode.Error()}
		return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
	}
	r.storeTunnel(req.TunnelID, &tunnelSession{
		key:          hopKey,
		nextPeerID:   rec.NextPeerID,
		nextTunnelID: append([]byte(nil), rec.NextTunnelID...),
		expiresAtMs:  rec.ExpiresAtMs,
	})
	reply := tunnel.BuildReply{V: 1, TunnelID: req.TunnelID, HopIndex: req.HopIndex, Status: "OK"}
	r.log.Event("info", "tunnel", "tunnel.build.ok", fromPeerID, "", "")
	return [][]byte{r.buildTunnelReply(fromPeerID, reply)}, nil
}

func (r *Runtime) handleTunnelData(fromPeerID string, payload []byte) ([][]byte, error) {
	req, err := tunnel.DecodeData(payload)
	if err != nil || req.V != 1 || len(req.TunnelID) != 16 {
		return nil, nil
	}
	s, ok := r.loadTunnel(req.TunnelID)
	if !ok {
		return nil, nil
	}
	if s.expiresAtMs > 0 && timeutil.NowUnixMs() > s.expiresAtMs {
		return nil, nil
	}
	if req.Seq <= s.lastSeq {
		r.log.Event("warn", "tunnel", "tunnel.data.replay", fromPeerID, "", "ERR_TUNNEL_DATA_REPLAY")
		return nil, nil
	}
	s.lastSeq = req.Seq
	inner, err := tunnel.DecryptData(s.key, req.CT)
	if err != nil {
		return nil, nil
	}
	switch inner.Kind {
	case "padding":
		return nil, nil
	case "deliver":
		r.log.Event("info", "tunnel", "tunnel.data.deliver", fromPeerID, "", "")
		if len(inner.Garlic) > 0 {
			_ = r.handleGarlic(inner.Garlic)
		}
		return nil, nil
	case "forward":
		if s.nextPeerID == "" || len(inner.Inner) == 0 {
			return nil, nil
		}
		if len(inner.NextTunnelID) == 16 && !tunnel.KeysEqual(inner.NextTunnelID, s.nextTunnelID) {
			return nil, nil
		}
		envBytes := r.buildTunnelDataEnvelope(s.nextPeerID, inner.Inner)
		_, _ = r.forwardEnvelope(s.nextPeerID, envBytes)
		return nil, nil
	default:
		return nil, nil
	}
}

func (r *Runtime) buildTunnelReply(toPeerID string, reply tunnel.BuildReply) []byte {
	payload, err := tunnel.EncodeBuildReply(reply)
	if err != nil {
		return nil
	}
	encoded, err := r.buildSignedEnvelopeBytes(tunnel.BuildReplyType, toPeerID, payload, ttlMinuteMs())
	if err != nil {
		return nil
	}
	return encoded
}

func (r *Runtime) buildTunnelDataEnvelope(toPeerID string, dataBytes []byte) []byte {
	encoded, err := r.buildSignedEnvelopeBytes(tunnel.DataType, toPeerID, dataBytes, ttlMinuteMs())
	if err != nil {
		return nil
	}
	return encoded
}
