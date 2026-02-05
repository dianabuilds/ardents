package runtime

import (
	"time"

	"github.com/dianabuilds/ardents/internal/shared/envelope"
)

func (r *Runtime) handleTaskResponse(fromPeerID string, env *envelope.Envelope) [][]byte {
	taskID, nodeID, err := decodeTaskResponsePayload(env.Type, env.Payload, int64((10*time.Second)/time.Millisecond))
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
