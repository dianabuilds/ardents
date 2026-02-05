package runtime

import (
	"context"
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
			if env.ReplyTo != nil {
				_, _ = r.deliverEnvelope(context.Background(), DeliveryTarget{ServiceID: env.ReplyTo.ServiceID}, b)
			}
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
	req, err := decodeTaskRequestPayload(env.Payload)
	if err != nil {
		return nil, err
	}
	if r.metrics != nil {
		r.metrics.IncTaskRequested(req.JobType)
	}
	if dup, ok := r.handleTaskDedupV2(req, env); ok {
		return dup, nil
	}
	return r.dispatchTaskV2(req, env)
}

func (r *Runtime) handleTaskResponseV2(env *envelopev2.Envelope, kind string) error {
	taskID, _, err := decodeTaskResponsePayload(env.Type, env.Payload, 0)
	if err != nil {
		return err
	}
	if r.tasks != nil && taskID != "" {
		if b, err := env.Encode(); err == nil {
			r.tasks.StoreResponse(taskID, b)
		}
	}
	if r.log != nil {
		r.log.Event("info", "task", "task.response."+kind, "", env.MsgID, "")
	}
	return nil
}

func (r *Runtime) handleTaskDedupV2(req tasks.Request, env *envelopev2.Envelope) ([][]byte, bool) {
	if r.tasks == nil {
		return nil, false
	}
	if dup, errCode := r.tasks.Check(req.TaskID, req.ClientRequestID, env.Payload); errCode != "" {
		fail := r.buildTaskFailV2(req.TaskID, errCode, "", env)
		r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
		return fail, true
	} else if len(dup) > 0 {
		return dup, true
	}
	return nil, false
}

func (r *Runtime) dispatchTaskV2(req tasks.Request, env *envelopev2.Envelope) ([][]byte, error) {
	switch req.JobType {
	case "ai.chat.v1":
		return r.handleAIChatTaskV2(req, env), nil
	case dirquery.JobType:
		out, err := r.handleDirQueryV2(req, env)
		if err != nil {
			return r.failTaskV2(req, env, err.Error()), nil
		}
		r.storeTaskV2(req, env, out)
		return out, nil
	default:
		return r.failTaskV2(req, env, tasks.ErrTaskUnsupported.Error()), nil
	}
}

func (r *Runtime) handleAIChatTaskV2(req tasks.Request, env *envelopev2.Envelope) [][]byte {
	input, err := aichat.DecodeInput(req.Input, r.cfg.Limits.MaxPayloadBytes)
	if err != nil {
		code := err.Error()
		if code == aichat.ErrInputInvalid.Error() {
			code = "ERR_PAYLOAD_DECODE"
		}
		return r.failTaskV2(req, env, code)
	}
	nodeID, err := r.buildAITranscript(req.TaskID, input)
	if err != nil {
		return r.failTaskV2(req, env, err.Error())
	}
	accept := r.buildTaskAcceptV2(req.TaskID, env)
	result := r.buildTaskResultV2(req.TaskID, nodeID, env)
	if r.metrics != nil {
		r.metrics.IncTaskResult(req.JobType)
	}
	out := append(accept, result...)
	r.storeTaskV2(req, env, out)
	return out
}

func (r *Runtime) failTaskV2(req tasks.Request, env *envelopev2.Envelope, code string) [][]byte {
	fail := r.buildTaskFailV2(req.TaskID, code, "", env)
	r.storeTaskV2(req, env, fail)
	return fail
}

func (r *Runtime) storeTaskV2(req tasks.Request, env *envelopev2.Envelope, resps [][]byte) {
	if r.tasks == nil {
		return
	}
	r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, resps)
}

func (r *Runtime) buildTaskAcceptV2(taskID string, env *envelopev2.Envelope) [][]byte {
	return r.buildTaskResponseV2(tasks.AcceptType, env, func() ([]byte, error) {
		payload := tasks.Accept{
			V:      tasks.Version,
			TaskID: taskID,
			TSMs:   timeutil.NowUnixMs(),
		}
		return tasks.EncodeAccept(payload)
	})
}

func (r *Runtime) buildTaskResultV2(taskID string, nodeID string, env *envelopev2.Envelope) [][]byte {
	return r.buildTaskResponseV2(tasks.ResultType, env, func() ([]byte, error) {
		payload := tasks.Result{
			V:            tasks.Version,
			TaskID:       taskID,
			ResultNodeID: nodeID,
			TSMs:         timeutil.NowUnixMs(),
		}
		return tasks.EncodeResult(payload)
	})
}

func (r *Runtime) buildTaskFailV2(taskID string, code string, message string, env *envelopev2.Envelope) [][]byte {
	if r.metrics != nil {
		r.metrics.IncTaskFail(code)
		if code == tasks.ErrTaskTimeout.Error() {
			r.metrics.IncTaskTimeout()
		}
	}
	return r.buildTaskResponseV2(tasks.FailType, env, func() ([]byte, error) {
		payload := tasks.Fail{
			V:            tasks.Version,
			TaskID:       taskID,
			ErrorCode:    code,
			ErrorMessage: message,
			TSMs:         timeutil.NowUnixMs(),
		}
		return tasks.EncodeFail(payload)
	})
}

func (r *Runtime) buildTaskResponseV2(typ string, env *envelopev2.Envelope, encode func() ([]byte, error)) [][]byte {
	payloadBytes, err := encode()
	if err != nil {
		return nil
	}
	return r.wrapEnvelopeV2(typ, payloadBytes, env)
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
