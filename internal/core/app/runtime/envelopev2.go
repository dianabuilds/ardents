package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/aichat"
	"github.com/dianabuilds/ardents/internal/core/app/services/dirquery"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/envelopev2"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func (r *Runtime) handleEnvelopeV2(env *envelopev2.Envelope) ([][]byte, error) {
	if env == nil {
		return nil, nil
	}
	nowMs := timeutil.NowUnixMs()
	if err := env.ValidateBasic(nowMs); err != nil {
		return nil, err
	}
	switch env.Type {
	case tasks.RequestType:
		resps, err := r.handleTaskV2(env)
		if err != nil {
			return nil, err
		}
		for _, b := range resps {
			_ = r.deliverEnvelopeV2(env.ReplyTo, b)
		}
		return resps, nil
	case tasks.AcceptType:
		if err := r.handleTaskResponseV2(env, "accept"); err != nil {
			return nil, err
		}
		return nil, nil
	case tasks.ProgressType:
		if err := r.handleTaskResponseV2(env, "progress"); err != nil {
			return nil, err
		}
		return nil, nil
	case tasks.ResultType:
		if err := r.handleTaskResponseV2(env, "result"); err != nil {
			return nil, err
		}
		return nil, nil
	case tasks.FailType:
		if err := r.handleTaskResponseV2(env, "fail"); err != nil {
			return nil, err
		}
		return nil, nil
	case tasks.ReceiptType:
		if err := r.handleTaskResponseV2(env, "receipt"); err != nil {
			return nil, err
		}
		return nil, nil
	default:
		return nil, nil
	}
}

func (r *Runtime) handleTaskV2(env *envelopev2.Envelope) ([][]byte, error) {
	req, err := tasks.DecodeRequest(env.Payload)
	if err != nil {
		return nil, err
	}
	if req.V != tasks.Version || req.TaskID == "" || req.ClientRequestID == "" || req.JobType == "" || req.TSMs <= 0 {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(req.TaskID); err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(req.ClientRequestID); err != nil {
		return nil, errors.New("ERR_PAYLOAD_DECODE")
	}

	if r.tasks != nil {
		if dup, errCode := r.tasks.Check(req.TaskID, req.ClientRequestID, env.Payload); errCode != "" {
			fail := r.buildTaskFailV2(req.TaskID, errCode, "", env)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
			}
			return fail, nil
		} else if len(dup) > 0 {
			return dup, nil
		}
	}

	switch req.JobType {
	case "ai.chat.v1":
		input, err := aichat.DecodeInput(req.Input, r.cfg.Limits.MaxPayloadBytes)
		if err != nil {
			code := err.Error()
			if code == aichat.ErrInputInvalid.Error() {
				code = "ERR_PAYLOAD_DECODE"
			}
			fail := r.buildTaskFailV2(req.TaskID, code, "", env)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
			}
			return fail, nil
		}
		nodeID, err := r.buildAITranscript(req.TaskID, input)
		if err != nil {
			fail := r.buildTaskFailV2(req.TaskID, err.Error(), "", env)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
			}
			return fail, nil
		}
		accept := r.buildTaskAcceptV2(req.TaskID, env)
		result := r.buildTaskResultV2(req.TaskID, nodeID, env)
		out := append(accept, result...)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, out)
		}
		return out, nil
	case dirquery.JobType:
		out, err := r.handleDirQueryV2(req, env)
		if err != nil {
			fail := r.buildTaskFailV2(req.TaskID, err.Error(), "", env)
			if r.tasks != nil {
				r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
			}
			return fail, nil
		}
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, out)
		}
		return out, nil
	default:
		fail := r.buildTaskFailV2(req.TaskID, tasks.ErrTaskUnsupported.Error(), "", env)
		if r.tasks != nil {
			r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		}
		return fail, nil
	}
}

func (r *Runtime) handleTaskResponseV2(env *envelopev2.Envelope, kind string) error {
	switch env.Type {
	case tasks.AcceptType:
		acc, err := tasks.DecodeAccept(env.Payload)
		if err != nil || acc.V != tasks.Version || acc.TaskID == "" || acc.TSMs <= 0 {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := uuidv7.Validate(acc.TaskID); err != nil {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	case tasks.ProgressType:
		p, err := tasks.DecodeProgress(env.Payload)
		if err != nil || p.V != tasks.Version || p.TaskID == "" || p.TSMs <= 0 {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := uuidv7.Validate(p.TaskID); err != nil {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	case tasks.ResultType:
		res, err := tasks.DecodeResult(env.Payload)
		if err != nil || res.V != tasks.Version || res.TaskID == "" || res.TSMs <= 0 {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := uuidv7.Validate(res.TaskID); err != nil {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if res.ResultNodeID == "" {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	case tasks.FailType:
		f, err := tasks.DecodeFail(env.Payload)
		if err != nil || f.V != tasks.Version || f.TaskID == "" || f.TSMs <= 0 {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := uuidv7.Validate(f.TaskID); err != nil {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if f.ErrorCode == "" {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	case tasks.ReceiptType:
		rc, err := tasks.DecodeReceipt(env.Payload)
		if err != nil || rc.V != tasks.Version || rc.TaskID == "" || rc.TSMs <= 0 {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := uuidv7.Validate(rc.TaskID); err != nil {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	default:
		return nil
	}
	if r.tasks != nil {
		if b, err := env.Encode(); err == nil {
			r.tasks.StoreResponse(getTaskIDFromResponse(env), b)
		}
	}
	if r.log != nil {
		r.log.Event("info", "task", "task.response."+kind, "", env.MsgID, "")
	}
	return nil
}

func getTaskIDFromResponse(env *envelopev2.Envelope) string {
	switch env.Type {
	case tasks.AcceptType:
		if a, err := tasks.DecodeAccept(env.Payload); err == nil {
			return a.TaskID
		}
	case tasks.ProgressType:
		if p, err := tasks.DecodeProgress(env.Payload); err == nil {
			return p.TaskID
		}
	case tasks.ResultType:
		if r, err := tasks.DecodeResult(env.Payload); err == nil {
			return r.TaskID
		}
	case tasks.FailType:
		if f, err := tasks.DecodeFail(env.Payload); err == nil {
			return f.TaskID
		}
	case tasks.ReceiptType:
		if r, err := tasks.DecodeReceipt(env.Payload); err == nil {
			return r.TaskID
		}
	}
	return ""
}

func (r *Runtime) buildTaskAcceptV2(taskID string, env *envelopev2.Envelope) [][]byte {
	payload := tasks.Accept{
		V:      tasks.Version,
		TaskID: taskID,
		TSMs:   timeutil.NowUnixMs(),
	}
	payloadBytes, err := tasks.EncodeAccept(payload)
	if err != nil {
		return nil
	}
	return r.wrapEnvelopeV2(tasks.AcceptType, payloadBytes, env)
}

func (r *Runtime) buildTaskResultV2(taskID string, nodeID string, env *envelopev2.Envelope) [][]byte {
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
	return r.wrapEnvelopeV2(tasks.ResultType, payloadBytes, env)
}

func (r *Runtime) buildTaskFailV2(taskID string, code string, message string, env *envelopev2.Envelope) [][]byte {
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
	return r.wrapEnvelopeV2(tasks.FailType, payloadBytes, env)
}

func (r *Runtime) wrapEnvelopeV2(typ string, payload []byte, env *envelopev2.Envelope) [][]byte {
	if env == nil || env.ReplyTo == nil || env.ReplyTo.ServiceID == "" {
		return nil
	}
	msgID, err := uuidv7.New()
	if err != nil {
		return nil
	}
	out := envelopev2.Envelope{
		V:     envelopev2.Version,
		MsgID: msgID,
		Type:  typ,
		From: envelopev2.From{
			IdentityID: r.identity.ID,
		},
		To: envelopev2.To{
			ServiceID: env.ReplyTo.ServiceID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if r.identity.PrivateKey != nil && r.identity.ID != "" {
		if err := out.Sign(r.identity.PrivateKey); err != nil {
			return nil
		}
	}
	encoded, err := out.Encode()
	if err != nil {
		return nil
	}
	return [][]byte{encoded}
}
