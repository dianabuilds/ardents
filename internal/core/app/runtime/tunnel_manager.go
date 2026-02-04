package runtime

import (
	"context"
	"crypto/rand"
	"errors"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/netdb"
	"github.com/dianabuilds/ardents/internal/core/domain/tunnel"
	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/shared/conv"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type tunnelHop struct {
	peerID   string
	tunnelID []byte
	key      []byte
	seq      uint64
}

type tunnelPath struct {
	direction     string
	hops          []tunnelHop
	createdAtMs   int64
	expiresAtMs   int64
	lastUsedMs    int64
	paddingCancel context.CancelFunc
	mu            sync.Mutex
}

func (p *tunnelPath) reserveNextSeqs() []uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]uint64, len(p.hops))
	for i := range p.hops {
		p.hops[i].seq++
		out[i] = p.hops[i].seq
	}
	return out
}

func (p *tunnelPath) markUsed(nowMs int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastUsedMs = nowMs
}

func (p *tunnelPath) idleFor(nowMs int64) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastUsedMs == 0 {
		return time.Duration(1 << 62)
	}
	return time.Duration(nowMs-p.lastUsedMs) * time.Millisecond
}

func (r *Runtime) startTunnelManager(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.tunnelMgrMu.Lock()
	if r.tunnelMgrCancel != nil {
		r.tunnelMgrCancel()
	}
	tctx, cancel := context.WithCancel(ctx)
	r.tunnelMgrCancel = cancel
	r.tunnelMgrMu.Unlock()

	go func() {
		r.ensureTunnels(tctx)
		params := r.tunnelParams()
		rotation := time.Duration(params.RotationMs) * time.Millisecond
		if rotation <= 0 {
			rotation = 10 * time.Minute
		}
		ticker := time.NewTicker(rotation)
		defer ticker.Stop()
		for {
			select {
			case <-tctx.Done():
				r.stopAllPadding()
				return
			case <-ticker.C:
				r.rotateTunnels(tctx)
			}
		}
	}()
}

func (r *Runtime) tunnelParams() reseed.TunnelParams {
	params := r.reseedParams.Tunnels
	if params.HopCountDefault == 0 {
		params.HopCountDefault = 3
	}
	if params.HopCountMin == 0 {
		params.HopCountMin = 2
	}
	if params.HopCountMax == 0 {
		params.HopCountMax = 5
	}
	if params.RotationMs == 0 {
		params.RotationMs = 600_000
	}
	if params.LeaseTTLMs == 0 {
		params.LeaseTTLMs = 600_000
	}
	if params.PaddingPolicy == "" {
		params.PaddingPolicy = "basic.v1"
	}
	return params
}

func (r *Runtime) ensureTunnels(ctx context.Context) {
	if r == nil {
		return
	}
	r.tunnelMgrMu.Lock()
	outCount := len(r.outboundTunnels)
	inCount := len(r.inboundTunnels)
	r.tunnelMgrMu.Unlock()
	if outCount > 0 && inCount > 0 {
		return
	}
	r.rotateTunnels(ctx)
}

func (r *Runtime) rotateTunnels(ctx context.Context) {
	if r == nil {
		return
	}
	r.stopAllPadding()
	r.tunnelMgrMu.Lock()
	r.outboundTunnels = nil
	r.inboundTunnels = nil
	r.tunnelMgrMu.Unlock()

	params := r.tunnelParams()
	hopCount := conv.ClampUint64ToInt(params.HopCountDefault)
	if hopCount < conv.ClampUint64ToInt(params.HopCountMin) {
		hopCount = conv.ClampUint64ToInt(params.HopCountMin)
	}
	if hopCount > conv.ClampUint64ToInt(params.HopCountMax) {
		hopCount = conv.ClampUint64ToInt(params.HopCountMax)
	}
	if hopCount < 2 {
		hopCount = 2
	}

	out, err := r.buildOutboundTunnel(hopCount)
	if err == nil && out != nil {
		r.tunnelMgrMu.Lock()
		r.outboundTunnels = append(r.outboundTunnels, out)
		r.tunnelMgrMu.Unlock()
		r.startPadding(ctx, out)
	}
	in, err := r.buildInboundTunnel(hopCount)
	if err == nil && in != nil {
		r.tunnelMgrMu.Lock()
		r.inboundTunnels = append(r.inboundTunnels, in)
		r.tunnelMgrMu.Unlock()
	}
	r.publishLocalServices()
}

func (r *Runtime) stopAllPadding() {
	r.tunnelMgrMu.Lock()
	defer r.tunnelMgrMu.Unlock()
	for _, t := range r.outboundTunnels {
		if t.paddingCancel != nil {
			t.paddingCancel()
		}
	}
}

func (r *Runtime) buildOutboundTunnel(hopCount int) (*tunnelPath, error) {
	hops, err := r.selectTunnelRouters(hopCount)
	if err != nil {
		return nil, err
	}
	return r.buildTunnelPath("outbound", hops)
}

func (r *Runtime) buildInboundTunnel(hopCount int) (*tunnelPath, error) {
	if hopCount < 2 {
		return nil, errors.New("ERR_TUNNEL_HOPS_INVALID")
	}
	hops, err := r.selectTunnelRouters(hopCount - 1)
	if err != nil {
		return nil, err
	}
	if r.peerID == "" || len(r.onionKey.Public) != 32 {
		return nil, errors.New("ERR_TUNNEL_SELF_UNAVAILABLE")
	}
	self := netdb.RouterInfo{
		V:        1,
		PeerID:   r.peerID,
		OnionPub: append([]byte(nil), r.onionKey.Public...),
	}
	hops = append(hops, self)
	return r.buildTunnelPath("inbound", hops)
}

func (r *Runtime) selectTunnelRouters(count int) ([]netdb.RouterInfo, error) {
	if r.netdb == nil {
		return nil, errors.New("ERR_NETDB_EMPTY")
	}
	nowMs := timeutil.NowUnixMs()
	routers := r.netdb.RoutersSnapshot(nowMs)
	filtered := make([]netdb.RouterInfo, 0, len(routers))
	for _, rinfo := range routers {
		if rinfo.PeerID == "" || rinfo.PeerID == r.peerID {
			continue
		}
		if r.IsBanned(rinfo.PeerID) {
			continue
		}
		if len(rinfo.OnionPub) != 32 {
			continue
		}
		filtered = append(filtered, rinfo)
	}
	if len(filtered) < count {
		return nil, errors.New("ERR_TUNNEL_NO_ROUTERS")
	}
	shuffle(filtered)
	return filtered[:count], nil
}

func (r *Runtime) buildTunnelPath(direction string, hops []netdb.RouterInfo) (*tunnelPath, error) {
	if len(hops) == 0 {
		return nil, errors.New("ERR_TUNNEL_HOPS_INVALID")
	}
	params := r.tunnelParams()
	nowMs := timeutil.NowUnixMs()
	expiresAtMs := nowMs + params.LeaseTTLMs
	path := newTunnelPath(direction, hops, nowMs, expiresAtMs)
	if err := r.buildTunnelRecords(path, hops, direction, expiresAtMs); err != nil {
		return nil, err
	}
	r.log.Event("info", "tunnel", "tunnel.build.path", direction, "", "")
	return path, nil
}

func newTunnelPath(direction string, hops []netdb.RouterInfo, nowMs int64, expiresAtMs int64) *tunnelPath {
	path := &tunnelPath{
		direction:   direction,
		createdAtMs: nowMs,
		expiresAtMs: expiresAtMs,
		lastUsedMs:  nowMs,
		hops:        make([]tunnelHop, len(hops)),
	}
	for i := range hops {
		path.hops[i] = tunnelHop{
			peerID:   hops[i].PeerID,
			tunnelID: newTunnelID(),
		}
	}
	return path
}

func (r *Runtime) buildTunnelRecords(path *tunnelPath, hops []netdb.RouterInfo, direction string, expiresAtMs int64) error {
	for i, hop := range hops {
		nextPeerID, nextTunnelID := nextHopInfo(hops, path, i)
		rec := tunnel.Record{
			V:            1,
			NextPeerID:   nextPeerID,
			NextTunnelID: nextTunnelID,
			ExpiresAtMs:  expiresAtMs,
			Flags:        tunnel.RecordFlags{IsGateway: false},
		}
		if hop.PeerID == r.peerID {
			if err := r.storeSelfTunnelHop(path, hop, i, nextPeerID, nextTunnelID, expiresAtMs); err != nil {
				return err
			}
			continue
		}
		if err := r.sendTunnelHopBuild(path, hop, i, rec, direction); err != nil {
			return err
		}
	}
	return nil
}

func nextHopInfo(hops []netdb.RouterInfo, path *tunnelPath, idx int) (string, []byte) {
	if idx >= len(hops)-1 {
		return "", nil
	}
	return hops[idx+1].PeerID, path.hops[idx+1].tunnelID
}

func (r *Runtime) storeSelfTunnelHop(path *tunnelPath, hop netdb.RouterInfo, idx int, nextPeerID string, nextTunnelID []byte, expiresAtMs int64) error {
	ephemeralPriv := make([]byte, 32)
	if _, err := rand.Read(ephemeralPriv); err != nil {
		return err
	}
	key, err := tunnel.DeriveHopKey(ephemeralPriv, hop.OnionPub, hop.PeerID, path.hops[idx].tunnelID)
	if err != nil {
		return err
	}
	path.hops[idx].key = key
	r.storeTunnel(path.hops[idx].tunnelID, &tunnelSession{
		key:          key,
		nextPeerID:   nextPeerID,
		nextTunnelID: append([]byte(nil), nextTunnelID...),
		expiresAtMs:  expiresAtMs,
	})
	return nil
}

func (r *Runtime) sendTunnelHopBuild(path *tunnelPath, hop netdb.RouterInfo, idx int, rec tunnel.Record, direction string) error {
	req := tunnel.BuildRequest{
		V:         1,
		Direction: direction,
		TunnelID:  path.hops[idx].tunnelID,
		HopIndex:  conv.ClampIntToUint64(idx),
	}
	req, ct, key, err := tunnel.EncryptRecord(req, rec, hop.OnionPub, hop.PeerID)
	if err != nil {
		return err
	}
	req.Record = ct
	if err := r.sendTunnelBuild(hop.PeerID, req); err != nil {
		return err
	}
	path.hops[idx].key = key
	return nil
}

func (r *Runtime) sendTunnelBuild(toPeerID string, req tunnel.BuildRequest) error {
	payload, err := tunnel.EncodeBuildRequest(req)
	if err != nil {
		return err
	}
	encoded, err := r.buildSignedEnvelopeBytes(tunnel.BuildType, toPeerID, payload, ttlMinuteMs())
	if err != nil {
		return err
	}
	_, err = r.forwardEnvelope(toPeerID, encoded)
	return err
}

func (r *Runtime) startPadding(ctx context.Context, path *tunnelPath) {
	if path == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	pctx, cancel := context.WithCancel(ctx)
	path.paddingCancel = cancel
	go func() {
		for {
			select {
			case <-pctx.Done():
				return
			default:
			}
			delay := jitter(2*time.Second, 500*time.Millisecond)
			time.Sleep(delay)
			nowMs := timeutil.NowUnixMs()
			if path.idleFor(nowMs) < delay {
				continue
			}
			data, err := r.buildPaddingData(path)
			if err != nil {
				continue
			}
			entry := path.hops[0]
			envBytes := r.buildTunnelDataEnvelope(entry.peerID, data)
			_, _ = r.forwardEnvelope(entry.peerID, envBytes)
		}
	}()
}

func (r *Runtime) buildPaddingData(path *tunnelPath) ([]byte, error) {
	if path == nil || len(path.hops) == 0 {
		return nil, errors.New("ERR_TUNNEL_PATH_EMPTY")
	}
	buckets := []int{512, 1024, 2048, 4096, 8192, 16384, 32768}
	seqs := path.reserveNextSeqs()
	ctLenForPadding := func(paddingLen int) (int, error) {
		inner := tunnel.Inner{V: 1, Kind: "padding"}
		if paddingLen > 0 {
			inner.Inner = make([]byte, paddingLen)
		}
		_, ctLen, err := r.buildTunnelDataWithSeqs(path, inner, seqs, false)
		if err != nil {
			return 0, err
		}
		return ctLen, nil
	}
	for _, target := range buckets {
		paddingLen, ok, err := findExactPaddingLen(ctLenForPadding, target)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		inner := tunnel.Inner{V: 1, Kind: "padding"}
		if paddingLen > 0 {
			inner.Inner = make([]byte, paddingLen)
			_, _ = rand.Read(inner.Inner)
		}
		data, _, err := r.buildTunnelDataWithSeqs(path, inner, seqs, true)
		return data, err
	}
	inner := tunnel.Inner{V: 1, Kind: "padding"}
	data, _, err := r.buildTunnelDataWithSeqs(path, inner, seqs, true)
	return data, err
}

func (r *Runtime) buildTunnelData(path *tunnelPath, inner tunnel.Inner) ([]byte, error) {
	if path == nil || len(path.hops) == 0 {
		return nil, errors.New("ERR_TUNNEL_PATH_EMPTY")
	}
	seqs := path.reserveNextSeqs()
	dataBytes, _, err := r.buildTunnelDataWithSeqs(path, inner, seqs, true)
	return dataBytes, err
}

func (r *Runtime) buildTunnelDataWithSeqs(path *tunnelPath, inner tunnel.Inner, seqs []uint64, markUsed bool) ([]byte, int, error) {
	if path == nil || len(path.hops) == 0 {
		return nil, 0, errors.New("ERR_TUNNEL_PATH_EMPTY")
	}
	if len(seqs) != len(path.hops) {
		return nil, 0, errors.New("ERR_TUNNEL_SEQ_INVALID")
	}
	nextInner := inner
	for i := len(path.hops) - 1; i >= 0; i-- {
		hop := &path.hops[i]
		seq := seqs[i]
		ct, err := tunnel.EncryptData(hop.key, nextInner)
		if err != nil {
			return nil, 0, err
		}
		data := tunnel.Data{
			V:        1,
			TunnelID: hop.tunnelID,
			Seq:      seq,
			CT:       ct,
		}
		dataBytes, err := tunnel.EncodeData(data)
		if err != nil {
			return nil, 0, err
		}
		if i == 0 {
			if markUsed {
				path.markUsed(timeutil.NowUnixMs())
			}
			return dataBytes, len(ct), nil
		}
		nextInner = tunnel.Inner{
			V:            1,
			Kind:         "forward",
			NextTunnelID: hop.tunnelID,
			Inner:        dataBytes,
		}
	}
	return nil, 0, errors.New("ERR_TUNNEL_BUILD_FAILED")
}

func findExactPaddingLen(ctLen func(paddingLen int) (int, error), target int) (int, bool, error) {
	if target <= 0 {
		return 0, false, nil
	}
	lo := 0
	loLen, err := ctLen(lo)
	if err != nil {
		return 0, false, err
	}
	if loLen > target {
		return 0, false, nil
	}
	hi := 1
	for hi < target {
		n, err := ctLen(hi)
		if err != nil {
			return 0, false, err
		}
		if n >= target {
			break
		}
		hi *= 2
	}
	if hi > target {
		hi = target
	}
	hiLen, err := ctLen(hi)
	if err != nil {
		return 0, false, err
	}
	if hiLen < target {
		return 0, false, nil
	}

	// Find the smallest paddingLen with ctLen >= target (monotonic non-decreasing).
	l, r := lo, hi
	for l < r {
		m := (l + r) / 2
		n, err := ctLen(m)
		if err != nil {
			return 0, false, err
		}
		if n < target {
			l = m + 1
		} else {
			r = m
		}
	}
	// Local scan around the boundary to hit exact size if it exists.
	start := l - 8
	if start < 0 {
		start = 0
	}
	end := l + 8
	if end > hi {
		end = hi
	}
	for pad := start; pad <= end; pad++ {
		n, err := ctLen(pad)
		if err != nil {
			return 0, false, err
		}
		if n == target {
			return pad, true, nil
		}
	}
	return 0, false, nil
}

func newTunnelID() []byte {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return id
}

func jitter(base time.Duration, delta time.Duration) time.Duration {
	if delta <= 0 {
		return base
	}
	n := randInt63n(int64(delta)*2) - int64(delta)
	return base + time.Duration(n)
}

var randSrc = mrand.New(mrand.NewSource(time.Now().UnixNano())) // #nosec G404 -- non-crypto jitter/shuffle.
var randMu sync.Mutex

func randInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	randMu.Lock()
	defer randMu.Unlock()
	return randSrc.Int63n(n)
}

func shuffle(items []netdb.RouterInfo) {
	randMu.Lock()
	defer randMu.Unlock()
	randSrc.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
}
