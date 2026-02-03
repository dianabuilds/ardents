package runtime

func (r *Runtime) capture(dir string, peerID string, data []byte) {
	if r == nil || r.pcap == nil {
		return
	}
	r.pcap.Write(dir, peerID, data)
}

func (r *Runtime) captureOutbound(peerID string, resps [][]byte) {
	if r == nil || r.pcap == nil {
		return
	}
	for _, b := range resps {
		if len(b) == 0 {
			continue
		}
		r.pcap.Write("out", peerID, b)
	}
}
