package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/runtime"
	"github.com/dianabuilds/ardents/internal/core/app/services/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/onionkey"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const simPowDifficulty = 16

func buildEnvelopeV1(sender *runtime.Runtime, toPeerID string, typ string, payload []byte) ([]byte, error) {
	if sender == nil {
		return nil, errors.New("ERR_SIM_SENDER_NIL")
	}
	msgID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  typ,
		From: envelope.From{
			PeerID:     sender.PeerID(),
			IdentityID: sender.IdentityID(),
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
	stamp, err := pow.Generate(sub, simPowDifficulty)
	if err != nil {
		return nil, err
	}
	env.Pow = stamp
	if env.From.IdentityID != "" {
		if err := env.Sign(sender.IdentityPrivateKey()); err != nil {
			return nil, err
		}
	}
	return env.Encode()
}

func getNetDBReply(resps [][]byte) (netdb.Reply, bool) {
	for _, resp := range resps {
		env, err := envelope.DecodeEnvelope(resp)
		if err != nil || env.Type != netdb.ReplyType {
			continue
		}
		reply, err := netdb.DecodeReply(env.Payload)
		if err != nil {
			continue
		}
		return reply, true
	}
	return netdb.Reply{}, false
}

func getAckStatus(resps [][]byte) (string, string) {
	for _, resp := range resps {
		env, err := envelope.DecodeEnvelope(resp)
		if err != nil || env.Type != "ack.v1" {
			continue
		}
		payload, err := ack.Decode(env.Payload)
		if err != nil {
			continue
		}
		return payload.Status, payload.ErrorCode
	}
	return "", ""
}

func dhtKey(typ, address string) [32]byte {
	b := []byte(typ + "\x00" + address)
	return sha256.Sum256(b)
}

func peelTunnelToGarlic(data []byte, snap *runtime.TunnelPathSnapshot) ([]byte, error) {
	if len(data) == 0 || snap == nil {
		return nil, errors.New("ERR_SIM_TUNNEL_EMPTY")
	}
	curr := data
	for i := 0; i < len(snap.HopKeys); i++ {
		decoded, err := tunnel.DecodeData(curr)
		if err != nil {
			return nil, err
		}
		inner, err := tunnel.DecryptData(snap.HopKeys[i], decoded.CT)
		if err != nil {
			return nil, err
		}
		switch inner.Kind {
		case "deliver":
			if len(inner.Garlic) == 0 {
				return nil, errors.New("ERR_SIM_GARLIC_EMPTY")
			}
			return inner.Garlic, nil
		case "forward":
			if len(inner.Inner) == 0 {
				return nil, errors.New("ERR_SIM_GARLIC_MISSING")
			}
			curr = inner.Inner
		default:
			return nil, errors.New("ERR_SIM_GARLIC_UNEXPECTED")
		}
	}
	return nil, errors.New("ERR_SIM_GARLIC_INCOMPLETE")
}

func equalTunnelSnapshots(a *runtime.TunnelPathSnapshot, b *runtime.TunnelPathSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.HopPeerIDs) != len(b.HopPeerIDs) || len(a.HopTunnelIDs) != len(b.HopTunnelIDs) {
		return false
	}
	for i := range a.HopPeerIDs {
		if a.HopPeerIDs[i] != b.HopPeerIDs[i] {
			return false
		}
		if !bytes.Equal(a.HopTunnelIDs[i], b.HopTunnelIDs[i]) {
			return false
		}
	}
	return true
}

func p95(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]int64(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(float64(len(cp)-1) * 0.95)
	return cp[idx]
}

func onionkeyFrom(priv []byte, pub []byte) onionkey.Keypair {
	return onionkey.Keypair{
		Private: append([]byte(nil), priv...),
		Public:  append([]byte(nil), pub...),
	}
}
