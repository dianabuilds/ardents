package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/delivery"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
)

var (
	ErrAckTimeout  = errors.New("ERR_ACK_TIMEOUT")
	ErrAckRejected = errors.New("ERR_ACK_REJECTED")
)

func (r *Runtime) sendEnvelopeWithRetry(ctx context.Context, addr string, peerID string, envBytes []byte, ackTimeout time.Duration, maxRetries int) ([]byte, error) {
	if r == nil || r.dial == nil {
		return nil, errors.New("ERR_RELAY_NEXT_HOP_UNREACHABLE")
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if ackTimeout <= 0 {
		ackTimeout = 1500 * time.Millisecond
	}
	msgID := ""
	if env, err := envelope.DecodeEnvelope(envBytes); err == nil {
		msgID = env.MsgID
		if r.tracker != nil && msgID != "" {
			r.tracker.Set(delivery.Record{MsgID: msgID, Status: delivery.StatusSent})
		}
	}
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		attemptCtx := ctx
		var cancel context.CancelFunc
		if ctx == nil {
			attemptCtx = context.Background()
		}
		attemptCtx, cancel = context.WithTimeout(attemptCtx, ackTimeout)
		start := time.Now()
		if r.metrics != nil {
			r.metrics.IncNetOutbound()
		}
		ackBytes, err := r.dial.SendEnvelope(attemptCtx, addr, peerID, envBytes, r.cfg.Limits.MaxMsgBytes)
		if r.metrics != nil {
			r.metrics.DecNetOutbound()
		}
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || err.Error() == "deadline exceeded" {
				lastErr = ErrAckTimeout
			} else {
				lastErr = err
			}
			continue
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
		if p.Status == "OK" || p.Status == "DUPLICATE" {
			if r.metrics != nil {
				r.metrics.ObserveAckLatency(conv.ClampInt64ToUint64(int64(time.Since(start) / time.Millisecond)))
			}
			if r.tracker != nil && msgID != "" {
				r.tracker.Set(delivery.Record{MsgID: msgID, Status: delivery.StatusAcked})
			}
			return ackBytes, nil
		}
		if p.Status == "REJECTED" {
			if r.tracker != nil && msgID != "" {
				r.tracker.Set(delivery.Record{MsgID: msgID, Status: delivery.StatusRejected, ErrorCode: p.ErrorCode})
			}
			if p.ErrorCode != "" {
				return nil, errors.New(p.ErrorCode)
			}
			return nil, ErrAckRejected
		}
		lastErr = ErrAckRejected
	}
	if r.tracker != nil && msgID != "" {
		code := ""
		if lastErr != nil {
			code = lastErr.Error()
		}
		r.tracker.Set(delivery.Record{MsgID: msgID, Status: delivery.StatusFailed, ErrorCode: code})
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrAckTimeout
}
