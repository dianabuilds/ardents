package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/contentnode"
	"github.com/dianabuilds/ardents/internal/providers"
	"github.com/dianabuilds/ardents/internal/services/aichat"
	"github.com/dianabuilds/ardents/internal/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/services/serviceannounce"
	"github.com/dianabuilds/ardents/internal/services/servicedesc"
	"github.com/dianabuilds/ardents/internal/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const (
	ackType = "ack.v1"
)

func (r *Runtime) handleEnvelope(fromPeerID string, data []byte) (resps [][]byte, err error) {
	r.capture("in", fromPeerID, data)
	defer func() {
		r.captureOutbound(fromPeerID, resps)
	}()
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
	if r.IsBanned(fromPeerID) {
		r.metrics.IncMsgRejected("ERR_POW_INVALID")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_POW_INVALID", fromPeerID)}, nil
	}
	if env.Type == ackType {
		return nil, nil
	}
	if r.dedup.SeenWithTTL(env.MsgID, time.Duration(env.TTLMs)*time.Millisecond) {
		r.metrics.IncMsgRejected("ERR_DEDUP")
		return [][]byte{r.buildAck(env.MsgID, "DUPLICATE", "", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" && len(env.Sig) == 0 {
		r.metrics.IncMsgRejected("ERR_SIG_REQUIRED")
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_REQUIRED", fromPeerID)}, nil
	}
	if env.From.IdentityID != "" {
		if r.book.IsRevokedIdentity(env.From.IdentityID) {
			r.metrics.IncMsgRejected("ERR_ID_REVOKED")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_ID_REVOKED", fromPeerID)}, nil
		}
		if err := env.VerifySignature(env.From.IdentityID); err != nil {
			r.metrics.IncMsgRejected("ERR_SIG_INVALID")
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SIG_INVALID", fromPeerID)}, nil
		}
		trusted := r.book.IsTrustedIdentity(env.From.IdentityID, nowMs) && !r.book.IsDeprecatedIdentity(env.From.IdentityID)
		if !trusted {
			if env.Pow == nil {
				r.metrics.IncPowRequired()
				r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
				r.handlePowAbuse(fromPeerID)
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
			}
			env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
			if err := pow.Verify(env.Pow); err != nil {
				r.metrics.IncPowInvalid()
				r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
				r.handlePowAbuse(fromPeerID)
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
			}
			r.resetPowAbuse(fromPeerID)
		} else {
			r.resetPowAbuse(fromPeerID)
		}
	} else {
		if env.Pow == nil {
			r.metrics.IncPowRequired()
			r.metrics.IncMsgRejected(pow.ErrPowRequired.Error())
			r.handlePowAbuse(fromPeerID)
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowRequired.Error(), fromPeerID)}, nil
		}
		env.Pow.Subject = pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		if err := pow.Verify(env.Pow); err != nil {
			r.metrics.IncPowInvalid()
			r.metrics.IncMsgRejected(pow.ErrPowInvalid.Error())
			r.handlePowAbuse(fromPeerID)
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", pow.ErrPowInvalid.Error(), fromPeerID)}, nil
		}
		r.resetPowAbuse(fromPeerID)
	}
	if env.Type == relayType {
		r.metrics.IncMsgReceived(relayType)
		return r.handleRelay(fromPeerID, env, nowMs)
	}
	r.metrics.IncMsgReceived(env.Type)
	if env.Type == nodefetch.ProviderAnnounceType {
		rec, err := nodefetch.DecodeProviderAnnounce(env.Payload)
		if err != nil || rec.NodeID == "" {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if rec.V != nodefetch.Version {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		if rec.ProviderPeerID == "" {
			rec.ProviderPeerID = fromPeerID
		}
		if rec.ProviderPeerID != fromPeerID {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if rec.TSMs == 0 {
			rec.TSMs = env.TSMs
		}
		if rec.TTLMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if r.providers != nil {
			r.providers.Add(providers.ProviderRecord{
				V:              rec.V,
				NodeID:         rec.NodeID,
				ProviderPeerID: rec.ProviderPeerID,
				TSMs:           rec.TSMs,
				TTLMs:          rec.TTLMs,
			}, nowMs)
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	if env.Type == serviceannounce.Type {
		ann, err := serviceannounce.Decode(env.Payload)
		if err != nil || ann.ServiceID == "" || ann.DescriptorNodeID == "" {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if ann.V != 1 || ann.TTLMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
		}
		descBytes, err := r.FetchNode(context.Background(), ann.DescriptorNodeID)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if err := contentnode.VerifyBytes(descBytes, ann.DescriptorNodeID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		var node contentnode.Node
		if err := contentnode.Decode(descBytes, &node); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		body, err := servicedesc.Validate(node)
		if err != nil {
			if errors.Is(err, servicedesc.ErrServiceIDMismatch) {
				return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_ID_MISMATCH", fromPeerID)}, nil
			}
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if ann.ServiceID != body.ServiceID {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_ID_MISMATCH", fromPeerID)}, nil
		}
		if r.book.IsRevokedIdentity(node.Owner) {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_ID_REVOKED", fromPeerID)}, nil
		}
		if !r.book.IsTrustedIdentity(node.Owner, nowMs) || r.book.IsDeprecatedIdentity(node.Owner) {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_SERVICE_DESCRIPTOR_INVALID", fromPeerID)}, nil
		}
		if r.services != nil {
			if r.services.UpdateIfNewer(body.ServiceID, ann.DescriptorNodeID, node.CreatedAtMs, body, fromPeerID) {
				r.log.Event("info", "service", "service.descriptor.updated", body.ServiceID, ann.DescriptorNodeID, "")
			}
		}
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	if env.Type == tasks.RequestType {
		req, err := tasks.DecodeRequest(env.Payload)
		if err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if req.V != tasks.Version || req.TaskID == "" || req.ClientRequestID == "" || req.JobType == "" || req.TSMs <= 0 {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if err := uuidv7.Validate(req.TaskID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if err := uuidv7.Validate(req.ClientRequestID); err != nil {
			return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}, nil
		}
		if r.tasks != nil {
			if dup, errCode := r.tasks.Check(req.TaskID, req.ClientRequestID, env.Payload); errCode != "" {
				fail := r.buildTaskFail(req.TaskID, errCode, "", fromPeerID)
				return append([][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, fail...), nil
			} else if len(dup) > 0 {
				resps := make([][]byte, 0, len(dup)+1)
				resps = append(resps, r.buildAck(env.MsgID, "OK", "", fromPeerID))
				resps = append(resps, dup...)
				return resps, nil
			}
		}
		if req.JobType == "ai.chat.v1" {
			input, err := aichat.DecodeInput(req.Input, r.cfg.Limits.MaxPayloadBytes)
			if err != nil {
				code := err.Error()
				if code == aichat.ErrInputInvalid.Error() {
					code = "ERR_PAYLOAD_DECODE"
				}
				fail := r.buildTaskFail(req.TaskID, code, "", fromPeerID)
				resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
				resps = append(resps, fail...)
				if r.tasks != nil {
					r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
				}
				return resps, nil
			}
			nodeID, err := r.buildAITranscript(req.TaskID, input)
			if err != nil {
				fail := r.buildTaskFail(req.TaskID, err.Error(), "", fromPeerID)
				resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
				resps = append(resps, fail...)
				if r.tasks != nil {
					r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
				}
				return resps, nil
			}
			accept := r.buildTaskAccept(req.TaskID, fromPeerID)
			result := r.buildTaskResult(req.TaskID, nodeID, fromPeerID)
			resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
			resps = append(resps, accept...)
			resps = append(resps, result...)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, resps[1:])
			}
			return resps, nil
		}
		fail := r.buildTaskFail(req.TaskID, tasks.ErrTaskUnsupported.Error(), "", fromPeerID)
		resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
		resps = append(resps, fail...)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		}
		return resps, nil
	}
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
	if env.Type == "chat.msg.v1" {
		return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}, nil
	}
	return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_UNSUPPORTED_TYPE", fromPeerID)}, nil
}

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
	msgID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  tasks.FailType,
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
	msgID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  tasks.AcceptType,
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
	msgID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  tasks.ResultType,
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
