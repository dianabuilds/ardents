package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
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
	if r.sessionPeers != nil {
		if peerID, ok := r.sessionPeers.Lookup(nodeID); ok {
			if addr, ok := r.resolvePeerAddr(peerID); ok {
				bytes, err := r.fetchFromProvider(ctx, addr, peerID, nodeID)
				if err == nil {
					if r.store != nil {
						_ = r.store.Put(nodeID, bytes)
					}
					return bytes, nil
				}
			}
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

// FetchNodeFromPeer fetches a node directly from a known peer address.
func (r *Runtime) FetchNodeFromPeer(ctx context.Context, addr string, peerID string, nodeID string) ([]byte, error) {
	if addr == "" || peerID == "" {
		return nil, ErrProviderUnavailable
	}
	return r.fetchFromProvider(ctx, addr, peerID, nodeID)
}

func (r *Runtime) resolveProviderAddr(peerID string) (string, bool) {
	return r.resolvePeerAddr(peerID)
}

func (r *Runtime) fetchFromProvider(ctx context.Context, addr string, peerID string, nodeID string) ([]byte, error) {
	if r.dial == nil {
		return nil, ErrDialerUnavailable
	}
	envBytes, err := r.buildNodeFetchEnvelope(peerID, nodeID)
	if err != nil {
		return nil, err
	}
	r.capture("out", peerID, envBytes)
	ackBytes, respBytes, err := r.sendNodeFetchWithRetry(ctx, addr, peerID, envBytes)
	if err != nil {
		return nil, err
	}
	r.capture("in", peerID, ackBytes)
	if len(respBytes) > 0 {
		r.capture("in", peerID, respBytes)
	}
	return r.decodeNodeFetchResponse(respBytes, nodeID)
}

func (r *Runtime) buildNodeFetchEnvelope(peerID string, nodeID string) ([]byte, error) {
	reqBytes, err := nodefetch.EncodeRequest(nodefetch.Request{V: nodefetch.Version, NodeID: nodeID})
	if err != nil {
		return nil, err
	}
	return r.buildSignedEnvelopeBytes(nodefetch.RequestType, peerID, reqBytes, ttlMinuteMs())
}

func (r *Runtime) sendNodeFetchWithRetry(ctx context.Context, addr string, peerID string, envBytes []byte) ([]byte, []byte, error) {
	ackTimeout := 1500 * time.Millisecond
	maxRetries := 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		ackBytes, respBytes, err := r.sendNodeFetchOnce(ctx, addr, peerID, envBytes, ackTimeout)
		if err != nil {
			lastErr = err
			continue
		}
		if err := r.validateNodeFetchAck(ackBytes); err != nil {
			if errors.Is(err, nodefetch.ErrNodeNotFound) {
				return nil, nil, err
			}
			lastErr = err
			continue
		}
		if len(respBytes) == 0 {
			return nil, nil, nodefetch.ErrNodeNotFound
		}
		return ackBytes, respBytes, nil
	}
	if lastErr == nil {
		lastErr = errors.New("ERR_DELIVERY_FAILED")
	}
	return nil, nil, lastErr
}

func (r *Runtime) sendNodeFetchOnce(ctx context.Context, addr string, peerID string, envBytes []byte, ackTimeout time.Duration) ([]byte, []byte, error) {
	ackCtx, cancel := context.WithTimeout(ctx, ackTimeout)
	defer cancel()
	return r.dial.SendEnvelopeWithReply(ackCtx, addr, peerID, envBytes, r.cfg.Limits.MaxMsgBytes, shouldAcceptAck, 5*time.Second)
}

func shouldAcceptAck(b []byte) bool {
	ackEnv, err := envelope.DecodeEnvelope(b)
	if err != nil {
		return false
	}
	p, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		return false
	}
	return p.Status == "OK" || p.Status == "DUPLICATE"
}

func (r *Runtime) validateNodeFetchAck(ackBytes []byte) error {
	ackEnv, err := envelope.DecodeEnvelope(ackBytes)
	if err != nil {
		return err
	}
	p, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		return err
	}
	switch p.Status {
	case "OK", "DUPLICATE":
		return nil
	case "REJECTED":
		if p.ErrorCode != "" {
			return errors.New(p.ErrorCode)
		}
		return nodefetch.ErrNodeNotFound
	default:
		return errors.New("ERR_ACK_INVALID")
	}
}

func (r *Runtime) decodeNodeFetchResponse(respBytes []byte, nodeID string) ([]byte, error) {
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
