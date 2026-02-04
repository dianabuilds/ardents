package runtime

func (r *Runtime) observeConnError(peerID string, remoteAddr string, stage string, err error) {
	if r == nil || r.log == nil || err == nil {
		return
	}
	fields := map[string]any{
		"stage":       stage,
		"remote_addr": remoteAddr,
		"error":       err.Error(),
	}
	r.log.EventWithFields("warn", "net", "net.conn.error", peerID, "", fields)
}
