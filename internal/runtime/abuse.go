package runtime

import (
	"time"
)

func (r *Runtime) handlePowAbuse(peerID string) {
	if r == nil || r.powAbuse == nil {
		return
	}
	if count, reached := r.powAbuse.Inc(peerID); reached {
		window := time.Duration(r.cfg.Limits.BanWindowMs) * time.Millisecond
		r.Ban(peerID, window)
		r.log.Event("warn", "net", "peer.banned", peerID, "", "ERR_POW_INVALID")
		_ = count
	}
}

func (r *Runtime) resetPowAbuse(peerID string) {
	if r == nil || r.powAbuse == nil {
		return
	}
	r.powAbuse.Reset(peerID)
}
