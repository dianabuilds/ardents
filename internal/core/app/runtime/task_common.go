package runtime

import (
	"errors"

	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

func decodeTaskRequestPayload(payload []byte) (tasks.Request, error) {
	req, err := tasks.DecodeRequest(payload)
	if err != nil {
		return tasks.Request{}, err
	}
	if req.V != tasks.Version || req.TaskID == "" || req.ClientRequestID == "" || req.JobType == "" || req.TSMs <= 0 {
		return tasks.Request{}, errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(req.TaskID); err != nil {
		return tasks.Request{}, errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(req.ClientRequestID); err != nil {
		return tasks.Request{}, errors.New("ERR_PAYLOAD_DECODE")
	}
	return req, nil
}

func decodeTaskResponsePayload(envType string, payload []byte, maxFutureSkewMs int64) (taskID string, nodeID string, err error) {
	switch envType {
	case tasks.AcceptType:
		taskID, err = decodeTaskResponse(payload, func(b []byte) (uint64, string, int64, error) {
			acc, err := tasks.DecodeAccept(b)
			if err != nil {
				return 0, "", 0, err
			}
			return acc.V, acc.TaskID, acc.TSMs, nil
		}, maxFutureSkewMs)
		return taskID, "", err
	case tasks.ProgressType:
		taskID, err = decodeTaskResponse(payload, func(b []byte) (uint64, string, int64, error) {
			p, err := tasks.DecodeProgress(b)
			if err != nil {
				return 0, "", 0, err
			}
			return p.V, p.TaskID, p.TSMs, nil
		}, maxFutureSkewMs)
		return taskID, "", err
	case tasks.ResultType:
		taskID, err = decodeTaskResponse(payload, func(b []byte) (uint64, string, int64, error) {
			res, err := tasks.DecodeResult(b)
			if err != nil {
				return 0, "", 0, err
			}
			if res.ResultNodeID == "" {
				return 0, "", 0, errors.New("ERR_PAYLOAD_DECODE")
			}
			return res.V, res.TaskID, res.TSMs, nil
		}, maxFutureSkewMs)
		if err != nil {
			return "", "", err
		}
		res, err := tasks.DecodeResult(payload)
		if err != nil {
			return "", "", err
		}
		return taskID, res.ResultNodeID, nil
	case tasks.FailType:
		taskID, err = decodeTaskResponse(payload, func(b []byte) (uint64, string, int64, error) {
			fail, err := tasks.DecodeFail(b)
			if err != nil {
				return 0, "", 0, err
			}
			if fail.ErrorCode == "" {
				return 0, "", 0, errors.New("ERR_PAYLOAD_DECODE")
			}
			return fail.V, fail.TaskID, fail.TSMs, nil
		}, maxFutureSkewMs)
		return taskID, "", err
	case tasks.ReceiptType:
		taskID, err = decodeTaskResponse(payload, func(b []byte) (uint64, string, int64, error) {
			rc, err := tasks.DecodeReceipt(b)
			if err != nil {
				return 0, "", 0, err
			}
			return rc.V, rc.TaskID, rc.TSMs, nil
		}, maxFutureSkewMs)
		return taskID, "", err
	default:
		return "", "", errors.New("ERR_PAYLOAD_DECODE")
	}
}

func decodeTaskResponse(payload []byte, decode func([]byte) (uint64, string, int64, error), maxFutureSkewMs int64) (string, error) {
	version, taskID, tsMs, err := decode(payload)
	if err != nil {
		return "", errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := validateTaskResponseMeta(version, taskID, tsMs, maxFutureSkewMs); err != nil {
		return "", errors.New("ERR_PAYLOAD_DECODE")
	}
	return taskID, nil
}

func validateTaskResponseMeta(version uint64, taskID string, tsMs int64, maxFutureSkewMs int64) error {
	if version != tasks.Version || taskID == "" || tsMs <= 0 {
		return errors.New("ERR_PAYLOAD_DECODE")
	}
	if err := uuidv7.Validate(taskID); err != nil {
		return errors.New("ERR_PAYLOAD_DECODE")
	}
	if maxFutureSkewMs > 0 {
		if tsMs > timeutil.NowUnixMs()+maxFutureSkewMs {
			return errors.New("ERR_PAYLOAD_DECODE")
		}
	}
	return nil
}
