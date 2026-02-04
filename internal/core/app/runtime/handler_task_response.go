package runtime

import (
	"errors"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func (r *Runtime) handleTaskResponse(fromPeerID string, env *envelope.Envelope) [][]byte {
	taskID, nodeID, err := decodeTaskResponseV1(env)
	if err != nil {
		return [][]byte{r.buildAck(env.MsgID, "REJECTED", "ERR_PAYLOAD_DECODE", fromPeerID)}
	}
	if taskID != "" && r.tasks != nil {
		if b, err := env.Encode(); err == nil {
			r.tasks.StoreResponse(taskID, b)
		}
	}
	if nodeID != "" && r.sessionPeers != nil {
		r.sessionPeers.Remember(nodeID, fromPeerID)
	}
	if r.log != nil {
		r.log.Event("info", "task", "task.response.v1", fromPeerID, env.MsgID, "")
	}
	return [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
}

func decodeTaskResponseV1(env *envelope.Envelope) (taskID string, nodeID string, err error) {
	if env == nil {
		return "", "", errors.New("ERR_PAYLOAD_DECODE")
	}
	switch env.Type {
	case tasks.AcceptType:
		acc, err := tasks.DecodeAccept(env.Payload)
		if err != nil {
			return "", "", err
		}
		if err := validateTaskResponseMetaV1(acc.V, acc.TaskID, acc.TSMs); err != nil {
			return "", "", err
		}
		return acc.TaskID, "", nil
	case tasks.ProgressType:
		p, err := tasks.DecodeProgress(env.Payload)
		if err != nil {
			return "", "", err
		}
		if err := validateTaskResponseMetaV1(p.V, p.TaskID, p.TSMs); err != nil {
			return "", "", err
		}
		return p.TaskID, "", nil
	case tasks.ResultType:
		res, err := tasks.DecodeResult(env.Payload)
		if err != nil {
			return "", "", err
		}
		if res.ResultNodeID == "" {
			return "", "", errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := validateTaskResponseMetaV1(res.V, res.TaskID, res.TSMs); err != nil {
			return "", "", err
		}
		return res.TaskID, res.ResultNodeID, nil
	case tasks.FailType:
		fail, err := tasks.DecodeFail(env.Payload)
		if err != nil {
			return "", "", err
		}
		if fail.ErrorCode == "" {
			return "", "", errors.New("ERR_PAYLOAD_DECODE")
		}
		if err := validateTaskResponseMetaV1(fail.V, fail.TaskID, fail.TSMs); err != nil {
			return "", "", err
		}
		return fail.TaskID, "", nil
	case tasks.ReceiptType:
		rc, err := tasks.DecodeReceipt(env.Payload)
		if err != nil {
			return "", "", err
		}
		if err := validateTaskResponseMetaV1(rc.V, rc.TaskID, rc.TSMs); err != nil {
			return "", "", err
		}
		return rc.TaskID, "", nil
	default:
		return "", "", errors.New("ERR_PAYLOAD_DECODE")
	}
}

func validateTaskResponseMetaV1(version uint64, taskID string, tsMs int64) error {
	if version != tasks.Version || taskID == "" || tsMs <= 0 {
		return errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(taskID); err != nil {
		return errors.New("ERR_PAYLOAD_DECODE")
	}
	if tsMs > timeutil.NowUnixMs()+int64((10*time.Second)/time.Millisecond) {
		return errors.New("ERR_PAYLOAD_DECODE")
	}
	return nil
}
