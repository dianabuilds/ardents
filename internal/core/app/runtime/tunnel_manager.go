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
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
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

func (p *tunnelPath) nextSeq(idx int) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hops[idx].seq++
	return p.hops[idx].seq
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
	hopCount := int(params.HopCountDefault)
	if hopCount < int(params.HopCountMin) {
		hopCount = int(params.HopCountMin)
	}
	if hopCount > int(params.HopCountMax) {
		hopCount = int(params.HopCountMax)
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
	for i, hop := range hops {
		nextPeerID := ""
		var nextTunnelID []byte
		if i < len(hops)-1 {
			nextPeerID = hops[i+1].PeerID
			nextTunnelID = path.hops[i+1].tunnelID
		}
		rec := tunnel.Record{
			V:            1,
			NextPeerID:   nextPeerID,
			NextTunnelID: nextTunnelID,
			ExpiresAtMs:  expiresAtMs,
			Flags:        tunnel.RecordFlags{IsGateway: false},
		}
		if hop.PeerID == r.peerID {
			ephemeralPriv := make([]byte, 32)
			if _, err := rand.Read(ephemeralPriv); err != nil {
				return nil, err
			}
			key, err := tunnel.DeriveHopKey(ephemeralPriv, hop.OnionPub, hop.PeerID, path.hops[i].tunnelID)
			if err != nil {
				return nil, err
			}
			path.hops[i].key = key
			r.storeTunnel(path.hops[i].tunnelID, &tunnelSession{
				key:          key,
				nextPeerID:   nextPeerID,
				nextTunnelID: append([]byte(nil), nextTunnelID...),
				expiresAtMs:  expiresAtMs,
			})
			continue
		}
		req := tunnel.BuildRequest{
			V:         1,
			Direction: direction,
			TunnelID:  path.hops[i].tunnelID,
			HopIndex:  uint64(i),
		}
		req, ct, key, err := tunnel.EncryptRecord(req, rec, hop.OnionPub, hop.PeerID)
		if err != nil {
			return nil, err
		}
		req.Record = ct
		if err := r.sendTunnelBuild(hop.PeerID, req); err != nil {
			return nil, err
		}
		path.hops[i].key = key
	}
	r.log.Event("info", "tunnel", "tunnel.build.path", direction, "", "")
	return path, nil
}

func (r *Runtime) sendTunnelBuild(toPeerID string, req tunnel.BuildRequest) error {
	payload, err := tunnel.EncodeBuildRequest(req)
	if err != nil {
		return err
	}
	msgID, err := uuidv7.New()
	if err != nil {
		return err
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  tunnel.BuildType,
		From: envelope.From{
			PeerID:     r.peerID,
			IdentityID: r.identity.ID,
		},
		To: envelope.To{
			PeerID: toPeerID,
		},
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if r.identity.PrivateKey != nil && r.identity.ID != "" {
		if err := env.Sign(r.identity.PrivateKey); err != nil {
			return err
		}
	} else {
		sub := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
		stamp, err := pow.Generate(sub, r.cfg.Pow.DefaultDifficulty)
		if err != nil {
			return err
		}
		env.Pow = stamp
	}
	encoded, err := env.Encode()
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
	padding := 0
	for _, target := range buckets {
		for padding = 0; padding <= target; padding++ {
			inner := tunnel.Inner{V: 1, Kind: "padding"}
			if padding > 0 {
				inner.Inner = make([]byte, padding)
				_, _ = rand.Read(inner.Inner)
			}
			data, err := r.buildTunnelData(path, inner)
			if err != nil {
				return nil, err
			}
			if decoded, err := tunnel.DecodeData(data); err == nil && len(decoded.CT) == target {
				return data, nil
			}
			if decoded, err := tunnel.DecodeData(data); err == nil && len(decoded.CT) > target {
				break
			}
		}
	}
	inner := tunnel.Inner{V: 1, Kind: "padding"}
	return r.buildTunnelData(path, inner)
}

func (r *Runtime) buildTunnelData(path *tunnelPath, inner tunnel.Inner) ([]byte, error) {
	if path == nil || len(path.hops) == 0 {
		return nil, errors.New("ERR_TUNNEL_PATH_EMPTY")
	}
	nextInner := inner
	for i := len(path.hops) - 1; i >= 0; i-- {
		hop := &path.hops[i]
		seq := path.nextSeq(i)
		ct, err := tunnel.EncryptData(hop.key, nextInner)
		if err != nil {
			return nil, err
		}
		data := tunnel.Data{
			V:        1,
			TunnelID: hop.tunnelID,
			Seq:      seq,
			CT:       ct,
		}
		dataBytes, err := tunnel.EncodeData(data)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			path.markUsed(timeutil.NowUnixMs())
			return dataBytes, nil
		}
		nextInner = tunnel.Inner{
			V:            1,
			Kind:         "forward",
			NextTunnelID: hop.tunnelID,
			Inner:        dataBytes,
		}
	}
	return nil, errors.New("ERR_TUNNEL_BUILD_FAILED")
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

var randSrc = mrand.New(mrand.NewSource(time.Now().UnixNano()))
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
