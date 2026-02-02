package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const (
	ackType = "ack.v1"
)

func (r *Runtime) handleEnvelope(fromPeerID string, data []byte) ([][]byte, error) {
	env, err := envelope.DecodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	nowMs := timeutil.NowUnixMs()
	if r.cfg.Limits.MaxMsgBytes > 0 && uint64(len(data)) > r.cfg.Limits.MaxMsgBytes {
		r.metrics.IncMsgRejected("ERR_MSG_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_MSG_TOO_LARGE", fromPeerID)}, nil
	}
	if r.cfg.Limits.MaxPayloadBytes > 0 && uint64(len(env.Payload)) > r.cfg.Limits.MaxPayloadBytes {
		r.metrics.IncMsgRejected("ERR_PAYLOAD_TOO_LARGE")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_TOO_LARGE", fromPeerID)}, nil
	}
	if err := env.ValidateBasic(nowMs); err != nil {
		r.metrics.IncMsgRejected(mapError(err))
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", mapError(err), fromPeerID)}, nil
	}
	if env.Type == ackType {
		return nil, nil
	}
	if r.dedup.Seen(env.MsgID) {
		r.metrics.IncMsgRejected("ERR_DEDUP")
		return [][]byte{r.buildAck(env.MsgID, "DUPLICATE", "", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" && len(env.Sig) == 0 {
		r.metrics.IncMsgRejected("ERR_SIG_REQUIRED")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_REQUIRED", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" {
		if err := env.VerifySignature(env.From.IdentityID); err != nil {
			r.metrics.IncMsgRejected("ERR_SIG_INVALID")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_INVALID", fromPeerID)}, nil
		}
		trusted := r.book.IsTrustedIdentity(env.From.IdentityID, nowMs)
		if !trusted {
			if env.Pow == nil {
				r.metrics.IncPowRequired()
				r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
			}
			env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
			if err := pow.Verify(env.Pow); err != nil {
				r.metrics.IncPowInvalid()
				r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
			}
		}
	} else {
		if env.Pow == nil {
			r.metrics.IncPowRequired()
			r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
		}
		env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		if err := pow.Verify(env.Pow); err != nil {
			r.metrics.IncPowInvalid()
			r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
		}
	}
	r.metrics.IncMsgReceived(env.Type)
	if env.Type == nodefetch.RequestType {
		req, err := nodefetch.DecodeRequest(env.Payload)
		if err != nil || req.NodeID == "" {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if r.store == nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", nodefetch.ErrNodeNotFound.Error(), fromPeerID)}, nil
		}
		nodeBytes, err := r.store.Get(req.NodeID)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", nodefetch.ErrNodeNotFound.Error(), fromPeerID)}, nil
		}
		resp := nodefetch.Response{V: nodefetch.Version, NodeBytes: nodeBytes}
		respBytes, err := nodefetch.EncodeResponse(resp)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		outID, err := uuidv7.New()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		outEnv := envelope.Envelope{
			V:     envelope.Version,
			MsgID: outID,
			Type:  nodefetch.ResponseType,
			From: envelope.From{
				PeerID: r.peerID,
			},
			To: envelope.To{
				PeerID: fromPeerID,
			},
			TSMs:    timeutil.NowUnixMs(),
			TTLMs:   int64((1 * time.Minute) / time.Millisecond),
			Payload: respBytes,
		}
		encoded, err := outEnv.Encode()
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		ackBytes := r.buildAck(env.MsgID, "OK", "", fromPeerID)
		return [][]byte{ackBytes, encoded}, nil
	}
	return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
}

func (r *Runtime) buildAck(msgID string, status string, code string, toPeerID string) []byte {
	ackPayload := ack.Payload{
		V:           ack.Version,
		AckForMsgID: msgID,
		Status:      status,
		ErrorCode:   code,
	}
	payloadBytes, err := ack.Encode(ackPayload)
	if err != nil {
		return nil
	}
	ackID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: ackID,
		Type:  ackType,
		From: envelope.From{
			PeerID:     r.peerID,
			IdentityID: "",
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payloadBytes,
	}
	encoded, err := env.Encode()
	if err != nil {
		return nil
	}
	return encoded
}

func mapError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, envelope.ErrExpired):
		return "ERR_TTL_EXPIRED"
	case errors.Is(err, envelope.ErrInvalidTTL):
		return "ERR_TTL_EXPIRED"
	case errors.Is(err, envelope.ErrInvalidMsgID):
		return "ERR_PAYLOAD_DECODE"
	case errors.Is(err, envelope.ErrInvalidFrom):
		return "ERR_PAYLOAD_DECODE"
	case errors.Is(err, envelope.ErrInvalidTo):
		return "ERR_PAYLOAD_DECODE"
	default:
		return "ERR_UNSUPPORTED_TYPE"
	}
}
