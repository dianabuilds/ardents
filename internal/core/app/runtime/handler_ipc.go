package runtime

import (
	"encoding/json"
	"errors"
	"net"
	"os"

	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
)

var (
	errIPCUnavailable      = errors.New("ERR_IPC_UNAVAILABLE")
	errIPCBadResponse      = errors.New("ERR_IPC_BAD_RESPONSE")
	errIPCNodeInvalid      = errors.New("ERR_NODE_INVALID")
	errIPCStoreUnavailable = errors.New("ERR_NODE_STORE_UNAVAILABLE")
)

func (r *Runtime) handleTaskIPC(req tasks.Request, env *envelope.Envelope, fromPeerID string) ([][]byte, bool) {
	handler := r.ipc.handler(req.JobType)
	if handler == nil {
		return nil, false
	}
	inputBytes, err := json.Marshal(req.Input)
	if err != nil {
		return r.buildIPCFailResps(env, req, fromPeerID, "ERR_PAYLOAD_DECODE", "", false), true
	}
	result, err := handler.requestTask(tasksRequest{
		TaskID:          req.TaskID,
		ClientRequestID: req.ClientRequestID,
		JobType:         req.JobType,
		Input:           inputBytes,
		TSMs:            req.TSMs,
	})
	if err != nil {
		code := errIPCUnavailable.Error()
		if errors.Is(err, errIPCBadResponse) {
			code = errIPCBadResponse.Error()
		}
		if r.metrics != nil {
			r.metrics.IncIPCErr(code)
			if isIPCTimeout(err) {
				r.metrics.IncIPCTimeout()
			}
		}
		return r.buildIPCFailResps(env, req, fromPeerID, code, "", false), true
	}
	if result.errorCode != "" {
		return r.buildIPCFailResps(env, req, fromPeerID, result.errorCode, result.errorMessage, true), true
	}
	nodeID, nodeBytes, err := r.decodeIPCNode(result.nodeBytes)
	if err != nil {
		return r.buildIPCFailResps(env, req, fromPeerID, err.Error(), "", false), true
	}
	if err := r.storeIPCNode(nodeID, nodeBytes); err != nil {
		return r.buildIPCFailResps(env, req, fromPeerID, err.Error(), "", false), true
	}
	return r.buildIPCResultResps(env, req, fromPeerID, nodeID), true
}

func (r *Runtime) buildIPCFailResps(env *envelope.Envelope, req tasks.Request, fromPeerID string, code string, message string, store bool) [][]byte {
	fail := r.buildTaskFail(req.TaskID, code, message, fromPeerID)
	resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
	resps = append(resps, fail...)
	if store && r.tasks != nil {
		r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, fail)
	}
	return resps
}

func isIPCTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (r *Runtime) decodeIPCNode(nodeBytes []byte) (string, []byte, error) {
	if len(nodeBytes) == 0 {
		return "", nil, errIPCBadResponse
	}
	var node contentnode.Node
	if err := contentnode.Decode(nodeBytes, &node); err != nil {
		return "", nil, errIPCNodeInvalid
	}
	if err := contentnode.Verify(&node); err != nil {
		return "", nil, errIPCNodeInvalid
	}
	encoded, nodeID, err := contentnode.EncodeWithCID(node)
	if err != nil {
		return "", nil, errIPCNodeInvalid
	}
	return nodeID, encoded, nil
}

func (r *Runtime) storeIPCNode(nodeID string, nodeBytes []byte) error {
	if r.store == nil {
		return errIPCStoreUnavailable
	}
	if err := r.store.Put(nodeID, nodeBytes); err != nil {
		return errIPCStoreUnavailable
	}
	return nil
}

func (r *Runtime) buildIPCResultResps(env *envelope.Envelope, req tasks.Request, fromPeerID string, nodeID string) [][]byte {
	return r.buildTaskSuccessResps(env, req, fromPeerID, nodeID)
}
