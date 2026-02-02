package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/contentnode"
	"github.com/dianabuilds/ardents/internal/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

var ErrProviderUnavailable = errors.New("ERR_PROVIDER_UNAVAILABLE")

func (r *Runtime) FetchNode(ctx context.Context, nodeID string) ([]byte, error) {
	if nodeID == "" {
		return nil, nodefetch.ErrNodeNotFound
	}
	if r.store != nil {
		if bytes, err := r.store.Get(nodeID); err == nil {
			return bytes, nil
		}
	}
	if r.providers == nil {
		return nil, nodefetch.ErrNodeNotFound
	}
	nowMs := timeutil.NowUnixMs()
	trustedPeers := r.book.TrustedPeers(nowMs)
	candidates := r.providers.Select(nodeID, trustedPeers, nowMs)
	if len(candidates) == 0 {
		return nil, nodefetch.ErrNodeNotFound
	}
	var lastErr error
	for _, cand := range candidates {
		addr, ok := r.resolveProviderAddr(cand.ProviderPeerID)
		if !ok {
			lastErr = ErrProviderUnavailable
			continue
		}
		bytes, err := r.fetchFromProvider(ctx, addr, cand.ProviderPeerID, nodeID)
		if err == nil {
			r.providers.MarkSuccess(cand.ProviderPeerID)
			if r.store != nil {
				_ = r.store.Put(nodeID, bytes)
			}
			return bytes, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = nodefetch.ErrNodeNotFound
	}
	return nil, lastErr
}

func (r *Runtime) resolveProviderAddr(peerID string) (string, bool) {
	for _, bp := range r.cfg.BootstrapPeers {
		if bp.PeerID != peerID {
			continue
		}
		if len(bp.Addrs) == 0 {
			return "", false
		}
		return stripSchemeLocal(bp.Addrs[0]), true
	}
	if r.quic != nil {
		if addr, ok := r.quic.PeerAddr(peerID); ok {
			return addr, true
		}
	}
	return "", false
}

func (r *Runtime) fetchFromProvider(ctx context.Context, addr string, peerID string, nodeID string) ([]byte, error) {
	if r.dial == nil {
		return nil, ErrDialerUnavailable
	}
	reqBytes, err := nodefetch.EncodeRequest(nodefetch.Request{V: nodefetch.Version, NodeID: nodeID})
	if err != nil {
		return nil, err
	}
	msgID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  nodefetch.RequestType,
		From: envelope.From{
			PeerID:     r.peerID,
			IdentityID: r.identity.ID,
		},
		To: envelope.To{
			PeerID: peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: reqBytes,
	}
	if r.identity.PrivateKey != nil && r.identity.ID != "" {
		if err := env.Sign(r.identity.PrivateKey); err != nil {
			return nil, err
		}
	} else {
		sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		stamp, err := pow.Generate(sub, r.cfg.Pow.DefaultDifficulty)
		if err != nil {
			return nil, err
		}
		env.Pow = stamp
	}
	envBytes, err := env.Encode()
	if err != nil {
		return nil, err
	}
	r.capture("out", peerID, envBytes)
	ackTimeout := 1500 * time.Millisecond
	maxRetries := 3
	var (
		ackBytes  []byte
		respBytes []byte
		lastErr   error
	)
	for attempt := 0; attempt < maxRetries; attempt++ {
		ackCtx, cancel := context.WithTimeout(ctx, ackTimeout)
		ackBytes, respBytes, err = r.dial.SendEnvelopeWithReply(ackCtx, addr, peerID, envBytes, r.cfg.Limits.MaxMsgBytes, func(b []byte) bool {
			ackEnv, err := envelope.DecodeEnvelope(b)
			if err != nil {
				return false
			}
			p, err := ack.Decode(ackEnv.Payload)
			if err != nil {
				return false
			}
			return p.Status == "OK" || p.Status == "DUPLICATE"
		}, 5*time.Second)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		r.capture("in", peerID, ackBytes)
		if len(respBytes) > 0 {
			r.capture("in", peerID, respBytes)
		}
		ackEnv, err := envelope.DecodeEnvelope(ackBytes)
		if err != nil {
			lastErr = err
			continue
		}
		p, err := ack.Decode(ackEnv.Payload)
		if err != nil {
			lastErr = err
			continue
		}
		switch p.Status {
		case "OK", "DUPLICATE":
		case "REJECTED":
			if p.ErrorCode != "" {
				return nil, errors.New(p.ErrorCode)
			}
			return nil, nodefetch.ErrNodeNotFound
		default:
			lastErr = errors.New("ERR_ACK_INVALID")
			continue
		}
		if len(respBytes) == 0 {
			return nil, nodefetch.ErrNodeNotFound
		}
		goto decodeResponse
	}
	if lastErr == nil {
		lastErr = errors.New("ERR_DELIVERY_FAILED")
	}
	return nil, lastErr

decodeResponse:
	respEnv, err := envelope.DecodeEnvelope(respBytes)
	if err != nil {
		return nil, err
	}
	if respEnv.Type != nodefetch.ResponseType {
		return nil, errors.New("ERR_UNSUPPORTED_TYPE")
	}
	resp, err := nodefetch.DecodeResponse(respEnv.Payload)
	if err != nil {
		return nil, err
	}
	if err := contentnode.VerifyBytes(resp.NodeBytes, nodeID); err != nil {
		switch {
		case errors.Is(err, contentnode.ErrCIDMismatch):
			return nil, nodefetch.ErrNodeCIDMismatch
		case errors.Is(err, contentnode.ErrInvalidNode):
			return nil, nodefetch.ErrNodeSigInvalid
		default:
			return nil, err
		}
	}
	return resp.NodeBytes, nil
}
