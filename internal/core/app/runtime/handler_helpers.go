package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const (
	ackType = "ack.v1"
)

func (r *Runtime) buildTaskFail(taskID string, code string, message string, toPeerID string) [][]byte {
	payload := tasks.Fail{
		V:            tasks.Version,
		TaskID:       taskID,
		ErrorCode:    code,
		ErrorMessage: message,
		TSMs:         timeutil.NowUnixMs(),
	}
	payloadBytes, err := tasks.EncodeFail(payload)
	if err != nil {
		return nil
	}
	return r.buildTaskEnvelope(tasks.FailType, payloadBytes, toPeerID)
}

func (r *Runtime) buildTaskAccept(taskID string, toPeerID string) [][]byte {
	payload := tasks.Accept{
		V:      tasks.Version,
		TaskID: taskID,
		TSMs:   timeutil.NowUnixMs(),
	}
	payloadBytes, err := tasks.EncodeAccept(payload)
	if err != nil {
		return nil
	}
	return r.buildTaskEnvelope(tasks.AcceptType, payloadBytes, toPeerID)
}

func (r *Runtime) buildTaskResult(taskID string, nodeID string, toPeerID string) [][]byte {
	payload := tasks.Result{
		V:            tasks.Version,
		TaskID:       taskID,
		ResultNodeID: nodeID,
		TSMs:         timeutil.NowUnixMs(),
	}
	payloadBytes, err := tasks.EncodeResult(payload)
	if err != nil {
		return nil
	}
	return r.buildTaskEnvelope(tasks.ResultType, payloadBytes, toPeerID)
}

func (r *Runtime) buildTaskEnvelope(taskType string, payloadBytes []byte, toPeerID string) [][]byte {
	msgID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  taskType,
		From: envelope.From{
			PeerID: r.peerID,
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
	return [][]byte{encoded}
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
	case errors.Is(err, envelope.ErrUnsupportedVersion):
		return "ERR_PAYLOAD_DECODE"
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
