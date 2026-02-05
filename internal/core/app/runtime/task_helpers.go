package runtime

import (
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
)

func (r *Runtime) buildTaskSuccessResps(env *envelope.Envelope, req tasks.Request, fromPeerID string, nodeID string) [][]byte {
	if r.metrics != nil {
		r.metrics.IncTaskResult(req.JobType)
	}
	accept := r.buildTaskAccept(req.TaskID, fromPeerID)
	result := r.buildTaskResult(req.TaskID, nodeID, fromPeerID)
	resps := [][]byte{r.buildAck(env.MsgID, "OK", "", fromPeerID)}
	resps = append(resps, accept...)
	resps = append(resps, result...)
	if r.tasks != nil {
		r.tasks.Store(req.TaskID, req.ClientRequestID, env.Payload, resps[1:])
	}
	return resps
}
