package runtime

import (
	"context"
	"errors"
	"time"

	"crypto/ed25519"

	"github.com/dianabuilds/ardents/internal/core/domain/relay"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/netaddr"
)

const relayType = "relay.packet.v1"

var ErrRelayUnsupported = errors.New("ERR_RELAY_UNSUPPORTED")

func (r *Runtime) handleRelay(fromPeerID string, env *envelope.Envelope, nowMs int64) ([][]byte, error) {
	pkt, err := relay.Decode(env.Payload)
	if err != nil || relay.Validate(pkt) != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_DECRYPT_FAILED", fromPeerID)}, nil
	}
	if nowMs > env.TSMs+pkt.TTLMs {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_TTL_EXPIRED", fromPeerID)}, nil
	}
	if r.transportKeys.PrivateKey == nil || r.transportKeys.PublicKey == nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_DECRYPT_FAILED", fromPeerID)}, nil
	}
	innerBytes, err := relay.OpenInner(r.transportKeys.PublicKey, r.transportKeys.PrivateKey, pkt.Inner)
	if err != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_DECRYPT_FAILED", fromPeerID)}, nil
	}
	if nextPkt, err := relay.Decode(innerBytes); err == nil && relay.Validate(nextPkt) == nil {
		if err := r.forwardRelayPacket(pkt.NextPeerID, innerBytes); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_NEXT_HOP_UNREACHABLE", fromPeerID)}, nil
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	if innerEnv, err := envelope.DecodeEnvelope(innerBytes); err == nil {
		ackBytes, err := r.forwardEnvelope(pkt.NextPeerID, innerBytes)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_NEXT_HOP_UNREACHABLE", fromPeerID)}, nil
		}
		if ackBytes != nil {
			_ = r.forwardAckToSender(innerEnv.From.PeerID, ackBytes)
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_RELAY_DECRYPT_FAILED", fromPeerID)}, nil
}

func (r *Runtime) SendViaRelay(ctx context.Context, relays []string, finalPeerID string, innerEnvBytes []byte) error {
	_ = ctx
	if len(relays) == 0 || len(relays) > 2 {
		return ErrRelayUnsupported
	}
	ttlMs := int64((1 * time.Minute) / time.Millisecond)
	inner := innerEnvBytes
	for i := len(relays) - 1; i >= 0; i-- {
		relayID := relays[i]
		next := finalPeerID
		if i < len(relays)-1 {
			next = relays[i+1]
		}
		pub, ok := r.peerPublicKey(relayID)
		if !ok {
			return errors.New("ERR_RELAY_PUBLIC_KEY_MISSING")
		}
		pkt, err := relay.Build(next, ttlMs, inner, pub)
		if err != nil {
			return err
		}
		inner, err = relay.Encode(pkt)
		if err != nil {
			return err
		}
	}
	envBytes, err := r.buildRelayEnvelope(relays[0], inner)
	if err != nil {
		return err
	}
	_, err = r.forwardEnvelope(relays[0], envBytes)
	return err
}

func (r *Runtime) forwardRelayPacket(nextPeerID string, relayPacketBytes []byte) error {
	envBytes, err := r.buildSignedEnvelopeBytes(relayType, nextPeerID, relayPacketBytes, ttlMinuteMs())
	if err != nil {
		return err
	}
	_, err = r.forwardEnvelope(nextPeerID, envBytes)
	return err
}

func (r *Runtime) buildRelayEnvelope(nextPeerID string, payload []byte) ([]byte, error) {
	return r.buildSignedEnvelopeBytes(relayType, nextPeerID, payload, ttlMinuteMs())
}

func (r *Runtime) forwardEnvelope(peerID string, envBytes []byte) ([]byte, error) {
	if r.relayForward != nil {
		if err := r.relayForward(peerID, envBytes); err != nil {
			return nil, err
		}
		return nil, nil
	}
	addr, ok := r.resolvePeerAddr(peerID)
	if !ok || r.dial == nil {
		return nil, errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	}
	ackBytes, err := r.sendEnvelopeWithRetry(context.Background(), addr, peerID, envBytes, 1500*time.Millisecond, 3)
	if err != nil {
		return nil, err
	}
	if len(ackBytes) == 0 {
		return nil, errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	}
	ackEnv, err := envelope.DecodeEnvelope(ackBytes)
	if err != nil {
		return nil, err
	}
	p, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		return nil, err
	}
	if p.Status != "OK" && p.Status != "DUPLICATE" {
		if p.ErrorCode != "" {
			return nil, errors.New(p.ErrorCode)
		}
		return nil, errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	}
	return ackBytes, nil
}

func (r *Runtime) resolvePeerAddr(peerID string) (string, bool) {
	for _, bp := range r.cfg.BootstrapPeers {
		if bp.PeerID != peerID {
			continue
		}
		if len(bp.Addrs) == 0 {
			return "", false
		}
		return netaddr.StripQUICScheme(bp.Addrs[0]), true
	}
	if r.quic != nil {
		if addr, ok := r.quic.PeerAddr(peerID); ok {
			return addr, true
		}
	}
	return "", false
}

func (r *Runtime) SetRelayForwarder(fn func(peerID string, envBytes []byte) error) {
	r.relayForward = fn
}

func (r *Runtime) peerPublicKey(peerID string) (ed25519.PublicKey, bool) {
	if r.quic != nil {
		if pub, ok := r.quic.PeerPublicKey(peerID); ok {
			return pub, true
		}
	}
	if r.dial != nil {
		if pub, ok := r.dial.PeerPublicKey(peerID); ok {
			return pub, true
		}
	}
	return nil, false
}

func (r *Runtime) forwardAckToSender(senderPeerID string, ackBytes []byte) error {
	if senderPeerID == "" || len(ackBytes) == 0 {
		return nil
	}
	addr, ok := r.resolvePeerAddr(senderPeerID)
	if !ok || r.dial == nil {
		return nil
	}
	_, err := r.dial.SendEnvelope(context.Background(), addr, senderPeerID, ackBytes, r.cfg.Limits.MaxMsgBytes)
	return err
}
