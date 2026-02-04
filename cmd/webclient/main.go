package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dianabuilds/ardents/internal/core/app/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/infra/discoverycache"
	"github.com/dianabuilds/ardents/internal/core/infra/reseed"
	"github.com/dianabuilds/ardents/internal/core/transport/cliutil"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/netaddr"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
	"github.com/dianabuilds/ardents/internal/shared/webtypes"
)

const (
	defaultJobType = "web.request.v1"
)

var (
	errClientBootstrapRequired    = errors.New("ERR_CLIENT_BOOTSTRAP_REQUIRED")
	errClientDiscoveryUnavailable = errors.New("ERR_CLIENT_DISCOVERY_UNAVAILABLE")
	errClientNoGatewayAvailable   = errors.New("ERR_CLIENT_NO_GATEWAY_AVAILABLE")
)

type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ",") }
func (h *headerList) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "request":
		requestCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage: webclient request --target <ufa> [flags]")
	fmt.Println("flags:")
	fmt.Println("  --addr <host:port>         target quic address (optional; if empty uses client.json)")
	fmt.Println("  --peer-id <peer_id>        verify remote peer id (optional)")
	fmt.Println("  --target <ufa>             alias|service_id|identity_id (UFA)")
	fmt.Println("  --job <job_type>           default web.request.v1")
	fmt.Println("  --method <METHOD>          default GET")
	fmt.Println("  --path <path>              default /")
	fmt.Println("  --header k=v               repeatable")
	fmt.Println("  --body-file <path>         optional request body")
	fmt.Println("  --timeout-ms <ms>          request timeout")
	fmt.Println("  --fetch-result             fetch result node and print response")
	fmt.Println("  --home <dir>               portable mode root (also Env: ARDENTS_HOME)")
	fmt.Println("  --config <path>            config file path")
	fmt.Println("  --client-config <path>     client config file path (default: XDG/ARDENTS_HOME)")
}

func requestCmd(args []string) {
	opts := parseRequestArgs(args)
	cfg, err := cliutil.LoadConfig(opts.home, opts.cfgPath)
	if err != nil {
		fatal(err)
	}
	dialer, err := quic.NewDialer(cfg)
	if err != nil {
		fatal(err)
	}
	book, err := cliutil.LoadAddressBook(opts.home)
	if err != nil {
		fatal(err)
	}
	serviceID, targetIdentityID, err := cliutil.ResolveServiceID(opts.target, opts.jobType, book, timeutil.NowUnixMs())
	if err != nil {
		fatal(err)
	}
	addr := opts.addr
	peerID := opts.peerID
	input, err := buildWebRequestInput(opts)
	if err != nil {
		fatal(err)
	}
	envBytes, err := buildRequestEnvelope(cfg, serviceID, opts.jobType, input)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.timeoutMs)*time.Millisecond)
	defer cancel()
	if addr == "" {
		addr, peerID, err = resolveGatewayAddr(ctx, dialer, opts, targetIdentityID)
		if err != nil {
			fatal(err)
		}
	}
	ack, resultEnv, err := sendAndWait(ctx, dialer, cfg, addr, peerID, envBytes)
	if err != nil {
		fatal(err)
	}
	printAck(ack)
	handleTaskResult(ctx, dialer, cfg, addr, peerID, resultEnv, opts.fetchResult)
}

func fetchAndPrint(ctx context.Context, dialer *quic.Dialer, cfg config.Config, addr string, peerID string, nodeID string) {
	if peerID == "" {
		fatal(errors.New("ERR_PEER_ID_REQUIRED"))
	}
	req := nodefetch.Request{V: nodefetch.Version, NodeID: nodeID}
	reqBytes, err := nodefetch.EncodeRequest(req)
	if err != nil {
		fatal(err)
	}
	envBytes, err := buildEnvelope(cfg, nodefetch.RequestType, reqBytes, envelope.To{PeerID: peerID})
	if err != nil {
		fatal(err)
	}
	_, respEnv, err := sendAndWait(ctx, dialer, cfg, addr, peerID, envBytes)
	if err != nil {
		fatal(err)
	}
	if respEnv == nil {
		fatal(errors.New("ERR_NODE_FETCH_FAILED"))
		return
	}
	resp := *respEnv
	if resp.Type != nodefetch.ResponseType {
		fatal(errors.New("ERR_NODE_FETCH_FAILED"))
		return
	}
	respPayload, err := nodefetch.DecodeResponse(resp.Payload)
	if err != nil {
		fatal(err)
	}
	if err := contentnode.VerifyBytes(respPayload.NodeBytes, nodeID); err != nil {
		fatal(err)
	}
	var node contentnode.Node
	if err := contentnode.Decode(respPayload.NodeBytes, &node); err != nil {
		fatal(err)
	}
	var body webtypes.ResponseV1
	bodyBytes, err := codec.Marshal(node.Body)
	if err != nil {
		fatal(err)
	}
	if err := codec.Unmarshal(bodyBytes, &body); err != nil {
		fatal(err)
	}
	out := map[string]any{
		"type":    node.Type,
		"status":  body.Status,
		"headers": body.Headers,
		"body":    string(body.Body),
	}
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(jsonBytes))
}

type ackStatus struct {
	Status    string
	ErrorCode string
}

func sendAndWait(ctx context.Context, dialer *quic.Dialer, cfg config.Config, addr string, peerID string, envBytes []byte) (*ackStatus, *envelope.Envelope, error) {
	dl := time.Duration(0)
	if deadline, ok := ctx.Deadline(); ok {
		dl = time.Until(deadline)
		if dl < 0 {
			dl = 0
		}
	}
	ackBytes, replyBytes, err := dialer.SendEnvelopeWithReplyUntil(
		ctx,
		addr,
		peerID,
		envBytes,
		cfg.Limits.MaxMsgBytes,
		nil,
		dl,
		func(frame []byte) bool {
			env, err := envelope.DecodeEnvelope(frame)
			if err != nil {
				return true
			}
			if env.Type == tasks.AcceptType || env.Type == tasks.ProgressType {
				return true
			}
			return false
		},
	)
	if err != nil {
		return nil, nil, err
	}
	ack := decodeAckStatus(ackBytes)
	if len(replyBytes) == 0 {
		return ack, nil, nil
	}
	env, err := envelope.DecodeEnvelope(replyBytes)
	if err != nil {
		return ack, nil, err
	}
	// Only return terminal / expected replies to keep behavior stable.
	switch env.Type {
	case tasks.ResultType, tasks.FailType, nodefetch.ResponseType:
		return ack, env, nil
	default:
		return ack, nil, errors.New("ERR_TASK_UNEXPECTED_REPLY")
	}
}

func decodeAckStatus(ackBytes []byte) *ackStatus {
	if len(ackBytes) == 0 {
		return nil
	}
	env, err := envelope.DecodeEnvelope(ackBytes)
	if err != nil || env.Type != "ack.v1" {
		return nil
	}
	ackPayload := struct {
		V         uint64 `cbor:"v"`
		MsgID     string `cbor:"msg_id"`
		Status    string `cbor:"status"`
		ErrorCode string `cbor:"error_code,omitempty"`
	}{}
	if err := codec.Unmarshal(env.Payload, &ackPayload); err != nil {
		return nil
	}
	return &ackStatus{Status: ackPayload.Status, ErrorCode: ackPayload.ErrorCode}
}

type requestOptions struct {
	addr        string
	peerID      string
	target      string
	jobType     string
	method      string
	path        string
	bodyFile    string
	timeoutMs   int
	fetchResult bool
	home        string
	cfgPath     string
	clientPath  string
	headers     headerList
}

func parseRequestArgs(args []string) requestOptions {
	fs := flag.NewFlagSet("request", flag.ExitOnError)
	opts := requestOptions{}
	fs.StringVar(&opts.addr, "addr", "", "target quic address host:port")
	fs.StringVar(&opts.peerID, "peer-id", "", "expected peer id (optional)")
	fs.StringVar(&opts.target, "target", "", "user-facing address (alias/service_id/identity_id)")
	fs.StringVar(&opts.jobType, "job", defaultJobType, "job type")
	fs.StringVar(&opts.method, "method", "GET", "http method")
	fs.StringVar(&opts.path, "path", "/", "relative path")
	fs.StringVar(&opts.bodyFile, "body-file", "", "request body file")
	fs.IntVar(&opts.timeoutMs, "timeout-ms", 10_000, "timeout in ms")
	fs.BoolVar(&opts.fetchResult, "fetch-result", false, "fetch result node")
	fs.StringVar(&opts.home, "home", "", "portable mode root (also Env: ARDENTS_HOME)")
	fs.StringVar(&opts.cfgPath, "config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	fs.StringVar(&opts.clientPath, "client-config", "", "path to client config file (default: XDG/ARDENTS_HOME)")
	fs.Var(&opts.headers, "header", "header k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	opts.addr = netaddr.StripQUICScheme(opts.addr)
	if opts.jobType == "" {
		fatal(errors.New("ERR_JOB_REQUIRED"))
	}
	if opts.target == "" {
		fatal(errors.New("ERR_UFA_REQUIRED"))
	}
	return opts
}

func loadClientConfig(home string, clientPath string) (config.ClientConfig, error) {
	dirs, err := cliutil.ResolveDirs(home)
	if err != nil {
		return config.ClientConfig{}, err
	}
	if clientPath == "" {
		clientPath = dirs.ClientConfigPath()
	}
	c, err := config.LoadClient(clientPath)
	if err != nil {
		if os.IsNotExist(err) {
			return config.ClientConfig{}, errClientBootstrapRequired
		}
		return config.ClientConfig{}, err
	}
	if len(c.BootstrapPeers) == 0 && !c.Reseed.Enabled {
		return config.ClientConfig{}, errClientBootstrapRequired
	}
	return c, nil
}

func resolveGatewayAddr(ctx context.Context, dialer *quic.Dialer, opts requestOptions, targetIdentityID string) (string, string, error) {
	cc, err := loadClientConfig(opts.home, opts.clientPath)
	if err != nil {
		return "", "", err
	}

	dirs, err := cliutil.ResolveDirs(opts.home)
	if err != nil {
		return "", "", err
	}
	nowMs := timeutil.NowUnixMs()

	cache, err := discoverycache.LoadOrInit(dirs.DiscoveryCachePath())
	if err != nil {
		cache = discoverycache.Cache{V: 1, Entries: []discoverycache.Entry{}}
	}
	addLimiter := discoverycache.NewAddLimiter(cc.Limits.AddRateLimit, cc.Limits.AddRateWindowMs)
	bootstrapTTL := int64((10 * time.Minute) / time.Millisecond)
	if cc.RefreshMs > 0 && cc.RefreshMs*2 > bootstrapTTL {
		bootstrapTTL = cc.RefreshMs * 2
	}

	ordered := orderClientPeers(cc.BootstrapPeers, targetIdentityID)
	cache = cache.UpsertBootstrapPeers(ordered, nowMs, bootstrapTTL, cc.Limits, addLimiter)
	_ = discoverycache.Save(dirs.DiscoveryCachePath(), cache)

	// If the user targets an explicit identity and that identity is in bootstrap_peers,
	// prefer dialing it directly. This keeps "target=identity" intuitive (no surprise routing
	// via some other gateway that happens to be "healthy").
	if targetIdentityID != "" {
		for _, p := range ordered {
			if p.IdentityID != targetIdentityID {
				continue
			}
			for _, addr := range p.Addrs {
				if err := dialer.DialAndHandshake(ctx, addr, p.PeerID); err != nil {
					cache = cache.MarkFail(p.PeerID, addr, nowMs, cc.Limits)
					continue
				}
				cache = cache.MarkOK(p.PeerID, addr, nowMs, nowMs+bootstrapTTL, cc.Limits)
				_ = discoverycache.Save(dirs.DiscoveryCachePath(), cache)
				return addr, p.PeerID, nil
			}
		}
		_ = discoverycache.Save(dirs.DiscoveryCachePath(), cache)
	}

	if addr, peerID, updated, ok := tryCacheCandidates(ctx, dialer, cache, nowMs, cc.Limits); ok {
		_ = discoverycache.Save(dirs.DiscoveryCachePath(), updated)
		return addr, peerID, nil
	}
	_ = discoverycache.Save(dirs.DiscoveryCachePath(), cache)

	if cc.Reseed.Enabled {
		bundle, err := reseed.FetchAndVerify(ctx, config.Reseed{
			Enabled:     true,
			NetworkID:   cc.Reseed.NetworkID,
			URLs:        append([]string(nil), cc.Reseed.URLs...),
			Authorities: append([]string(nil), cc.Reseed.Authorities...),
		})
		if err != nil {
			return "", "", errClientDiscoveryUnavailable
		}
		seedPeers := bundle.SeedPeers()
		for _, sp := range seedPeers {
			for _, addr := range sp.Addrs {
				cache = cache.UpsertCandidate(sp.PeerID, addr, "reseed", nowMs, nowMs+bootstrapTTL, cc.Limits, addLimiter)
				if err := dialer.DialAndHandshake(ctx, addr, sp.PeerID); err != nil {
					cache = cache.MarkFail(sp.PeerID, addr, nowMs, cc.Limits)
					continue
				}
				cache = cache.MarkOK(sp.PeerID, addr, nowMs, nowMs+bootstrapTTL, cc.Limits)
				_ = discoverycache.Save(dirs.DiscoveryCachePath(), cache)
				return addr, sp.PeerID, nil
			}
		}
	}

	return "", "", errClientNoGatewayAvailable
}

func orderClientPeers(peers []config.ClientPeer, targetIdentityID string) []config.ClientPeer {
	if len(peers) == 0 || targetIdentityID == "" {
		return peers
	}
	out := make([]config.ClientPeer, 0, len(peers))
	for _, p := range peers {
		if p.IdentityID == targetIdentityID {
			out = append(out, p)
		}
	}
	for _, p := range peers {
		if p.IdentityID != targetIdentityID {
			out = append(out, p)
		}
	}
	return out
}

func tryCacheCandidates(ctx context.Context, dialer *quic.Dialer, cache discoverycache.Cache, nowMs int64, lim config.ClientLimits) (string, string, discoverycache.Cache, bool) {
	cands := cache.Candidates(nowMs)
	for _, e := range cands {
		if err := dialer.DialAndHandshake(ctx, e.Addr, e.PeerID); err != nil {
			cache = cache.MarkFail(e.PeerID, e.Addr, nowMs, lim)
			continue
		}
		cache = cache.MarkOK(e.PeerID, e.Addr, nowMs, e.ExpiresAtMs, lim)
		return e.Addr, e.PeerID, cache, true
	}
	return "", "", cache, false
}

func buildWebRequestInput(opts requestOptions) (map[string]any, error) {
	reqBody, err := loadBody(opts.bodyFile)
	if err != nil {
		return nil, err
	}
	input := map[string]any{
		"v":      uint64(1),
		"method": strings.ToUpper(opts.method),
		"path":   opts.path,
	}
	if len(opts.headers) > 0 {
		input["headers"] = parseHeaders(opts.headers)
	}
	if len(reqBody) > 0 {
		input["body"] = reqBody
	}
	return input, nil
}

func buildRequestEnvelope(cfg config.Config, serviceID string, jobType string, input map[string]any) ([]byte, error) {
	taskID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	clientID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	task := tasks.Request{
		V:               tasks.Version,
		TaskID:          taskID,
		ClientRequestID: clientID,
		JobType:         jobType,
		Input:           input,
		TSMs:            timeutil.NowUnixMs(),
	}
	taskBytes, err := tasks.EncodeRequest(task)
	if err != nil {
		return nil, err
	}
	return buildEnvelope(cfg, tasks.RequestType, taskBytes, envelope.To{ServiceID: serviceID})
}

func printAck(ack *ackStatus) {
	if ack != nil {
		fmt.Println("ack:", ack.Status, ack.ErrorCode)
	}
}

func handleTaskResult(ctx context.Context, dialer *quic.Dialer, cfg config.Config, addr string, peerID string, resultEnv *envelope.Envelope, fetchResult bool) {
	if resultEnv == nil {
		fatal(errors.New("ERR_TASK_NO_RESULT"))
		return
	}
	result := *resultEnv
	switch result.Type {
	case tasks.ResultType:
		res, err := tasks.DecodeResult(result.Payload)
		if err != nil {
			fatal(err)
		}
		fmt.Println("result_node_id:", res.ResultNodeID)
		if fetchResult {
			fetchPeerID := peerID
			if fetchPeerID == "" {
				fetchPeerID = result.From.PeerID
			}
			fetchAndPrint(ctx, dialer, cfg, addr, fetchPeerID, res.ResultNodeID)
		}
	case tasks.FailType:
		fail, err := tasks.DecodeFail(result.Payload)
		if err != nil {
			fatal(err)
		}
		fatal(errors.New(fail.ErrorCode))
	default:
		fatal(errors.New("ERR_TASK_UNEXPECTED_REPLY"))
	}
}

func buildEnvelope(cfg config.Config, envType string, payload []byte, to envelope.To) ([]byte, error) {
	msgID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	keys, err := quic.LoadOrCreateKeyMaterial("")
	if err != nil {
		return nil, err
	}
	peerID, err := ids.NewPeerID(keys.PublicKey)
	if err != nil {
		return nil, err
	}
	env := envelope.Envelope{
		V:     envelope.Version,
		MsgID: msgID,
		Type:  envType,
		From: envelope.From{
			PeerID: peerID,
		},
		To:      to,
		TSMs:    timeutil.NowUnixMs(),
		TTLMs:   int64((1 * time.Minute) / time.Millisecond),
		Payload: payload,
	}
	if !env.To.HasExactlyOne() {
		return nil, errors.New("ERR_ENVELOPE_TO_INVALID")
	}
	subject := pow.Subject(env.MsgID, env.TSMs, env.From.PeerID)
	stamp, err := pow.Generate(subject, cfg.Pow.DefaultDifficulty)
	if err != nil {
		return nil, err
	}
	env.Pow = stamp
	return env.Encode()
}

func loadBody(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	return os.ReadFile(path) // #nosec G304 -- CLI user-supplied path.
}

func parseHeaders(list []string) map[string]string {
	out := map[string]string{}
	for _, entry := range list {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
