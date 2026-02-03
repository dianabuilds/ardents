package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/delivery"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

var ErrDialerUnavailable = errors.New("dialer unavailable")

type ChatMessage struct {
	V    uint64 `cbor:"v"`
	Text string `cbor:"text"`
}

func (r *Runtime) SendChat(ctx context.Context, addr string, peerID string, text string) ([]byte, error) {
	if r.dial == nil {
		return nil, ErrDialerUnavailable
	}
	payload, err := codec.Marshal(ChatMessage{V: 1, Text: text})
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
		Type:  "chat.msg.v1",
		From: envelope.From{
			PeerID:     r.peerID,
			IdentityID: r.identity.ID,
		},
		To: envelope.To{
			PeerID: peerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
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
	data, err := env.Encode()
	if err != nil {
		return nil, err
	}
	r.capture("out", peerID, data)
	r.tracker.Set(delivery.Record{MsgID: env.MsgID, Status: delivery.StatusSent})
	r.log.Event("info", "msg", "delivery.sent", peerID, env.MsgID, "")
	ackTimeout := 1500 * time.Millisecond
	maxRetries := 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		ackCtx, cancel := context.WithTimeout(ctx, ackTimeout)
		ackBytes, err := r.dial.SendEnvelope(ackCtx, stripSchemeLocal(addr), peerID, data, r.cfg.Limits.MaxMsgBytes)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		r.capture("in", peerID, ackBytes)
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
		r.metrics.ObserveAckLatency(uint64(timeutil.NowUnixMs() - env.TSMs))
		switch p.Status {
		case "OK", "DUPLICATE":
			r.tracker.Set(delivery.Record{MsgID: env.MsgID, Status: delivery.StatusAcked})
			r.log.Event("info", "msg", "delivery.acked", peerID, env.MsgID, "")
			return ackBytes, nil
		case "REJECTED":
			r.tracker.Set(delivery.Record{MsgID: env.MsgID, Status: delivery.StatusRejected, ErrorCode: p.ErrorCode})
			r.log.Event("warn", "msg", "delivery.rejected", peerID, env.MsgID, p.ErrorCode)
			return ackBytes, nil
		default:
			lastErr = errors.New("ERR_ACK_INVALID")
			continue
		}
	}
	r.tracker.Set(delivery.Record{MsgID: env.MsgID, Status: delivery.StatusFailed, ErrorCode: "ERR_DELIVERY_FAILED"})
	r.log.Event("warn", "msg", "delivery.failed", peerID, env.MsgID, "ERR_DELIVERY_FAILED")
	if lastErr == nil {
		lastErr = errors.New("ERR_DELIVERY_FAILED")
	}
	return nil, lastErr
}

func stripSchemeLocal(addr string) string {
	const prefix = "quic://"
	if len(addr) >= len(prefix) && addr[:len(prefix)] == prefix {
		return addr[len(prefix):]
	}
	return addr
}
