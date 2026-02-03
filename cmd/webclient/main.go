package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	quicgo "github.com/quic-go/quic-go"

	"github.com/dianabuilds/ardents/internal/core/app/services/nodefetch"
	"github.com/dianabuilds/ardents/internal/core/app/services/tasks"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/pow"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/uuidv7"
)

const (
	defaultJobType = "web.request.v1"
)

type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ",") }
func (h *headerList) Set(v string) error {
	*h = append(*h, v)
	return nil
}

type webResponseBody struct {
	V       uint64            `cbor:"v"`
	TaskID  string            `cbor:"task_id"`
	Status  uint16            `cbor:"status"`
	Headers map[string]string `cbor:"headers,omitempty"`
	Body    []byte            `cbor:"body,omitempty"`
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
	fmt.Println("usage: webclient request --addr <host:port> --service-id <service_id> [flags]")
	fmt.Println("flags:")
	fmt.Println("  --peer-id <peer_id>        verify remote peer id (optional)")
	fmt.Println("  --owner-id <identity_id>   compute service_id for web.request.v1")
	fmt.Println("  --service-id <service_id>  target service id (required unless --owner-id)")
	fmt.Println("  --job <job_type>           default web.request.v1")
	fmt.Println("  --method <METHOD>          default GET")
	fmt.Println("  --path <path>              default /")
	fmt.Println("  --header k=v               repeatable")
	fmt.Println("  --body-file <path>         optional request body")
	fmt.Println("  --timeout-ms <ms>          request timeout")
	fmt.Println("  --fetch-result             fetch result node and print response")
	fmt.Println("  --home <dir>               portable mode root (also Env: ARDENTS_HOME)")
	fmt.Println("  --config <path>            config file path")
}

func requestCmd(args []string) {
	opts := parseRequestArgs(args)
	cfg := loadConfig(opts.home, opts.cfgPath)
	addr, serviceID := resolveTarget(opts)
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
	ack, resultEnv, err := sendAndWait(ctx, cfg, addr, opts.peerID, envBytes)
	if err != nil {
		fatal(err)
	}
	printAck(ack)
	handleTaskResult(ctx, cfg, addr, opts.peerID, resultEnv, opts.fetchResult)
}

func fetchAndPrint(ctx context.Context, cfg config.Config, addr string, peerID string, nodeID string) {
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
	_, respEnv, err := sendAndWait(ctx, cfg, addr, peerID, envBytes)
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
	var body webResponseBody
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

func sendAndWait(ctx context.Context, cfg config.Config, addr string, peerID string, envBytes []byte) (*ackStatus, *envelope.Envelope, error) {
	conn, stream, err := dialAndHello(ctx, cfg, addr, peerID)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "")
	}()
	if err := quic.WriteFrame(stream, envBytes); err != nil {
		return nil, nil, err
	}
	return readAckAndResult(stream, cfg, ctx)
}

type requestOptions struct {
	addr        string
	peerID      string
	ownerID     string
	serviceID   string
	jobType     string
	method      string
	path        string
	bodyFile    string
	timeoutMs   int
	fetchResult bool
	home        string
	cfgPath     string
	headers     headerList
}

func parseRequestArgs(args []string) requestOptions {
	fs := flag.NewFlagSet("request", flag.ExitOnError)
	opts := requestOptions{}
	fs.StringVar(&opts.addr, "addr", "", "target quic address host:port")
	fs.StringVar(&opts.peerID, "peer-id", "", "expected peer id (optional)")
	fs.StringVar(&opts.ownerID, "owner-id", "", "owner identity id (optional)")
	fs.StringVar(&opts.serviceID, "service-id", "", "target service id")
	fs.StringVar(&opts.jobType, "job", defaultJobType, "job type")
	fs.StringVar(&opts.method, "method", "GET", "http method")
	fs.StringVar(&opts.path, "path", "/", "relative path")
	fs.StringVar(&opts.bodyFile, "body-file", "", "request body file")
	fs.IntVar(&opts.timeoutMs, "timeout-ms", 10_000, "timeout in ms")
	fs.BoolVar(&opts.fetchResult, "fetch-result", false, "fetch result node")
	fs.StringVar(&opts.home, "home", "", "portable mode root (also Env: ARDENTS_HOME)")
	fs.StringVar(&opts.cfgPath, "config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	fs.Var(&opts.headers, "header", "header k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if opts.addr == "" {
		fatal(errors.New("ERR_ADDR_REQUIRED"))
	}
	opts.addr = stripScheme(opts.addr)
	if opts.jobType == "" {
		fatal(errors.New("ERR_JOB_REQUIRED"))
	}
	if opts.serviceID == "" && opts.ownerID == "" {
		fatal(errors.New("ERR_SERVICE_REQUIRED"))
	}
	return opts
}

func loadConfig(home string, cfgPath string) config.Config {
	if home != "" {
		_ = os.Setenv(appdirs.EnvHome, home)
	}
	dirs, err := appdirs.Resolve(home)
	if err != nil {
		fatal(err)
	}
	if cfgPath == "" {
		cfgPath = dirs.ConfigPath()
	}
	cfg, err := config.LoadOrInit(cfgPath)
	if err != nil {
		fatal(err)
	}
	return cfg
}

func resolveTarget(opts requestOptions) (string, string) {
	if opts.serviceID != "" {
		return opts.addr, opts.serviceID
	}
	if err := ids.ValidateIdentityID(opts.ownerID); err != nil {
		fatal(err)
	}
	sid, err := ids.NewServiceID(opts.ownerID, opts.jobType)
	if err != nil {
		fatal(err)
	}
	return opts.addr, sid
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

func handleTaskResult(ctx context.Context, cfg config.Config, addr string, peerID string, resultEnv *envelope.Envelope, fetchResult bool) {
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
			fetchAndPrint(ctx, cfg, addr, peerID, res.ResultNodeID)
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

func dialAndHello(ctx context.Context, cfg config.Config, addr string, peerID string) (quicgo.Connection, quicgo.Stream, error) {
	addr, err := normalizeAddr(addr)
	if err != nil {
		return nil, nil, err
	}
	keys, tlsConf, quicConf, err := clientConfigs()
	if err != nil {
		return nil, nil, err
	}
	conn, err := quicgo.DialAddr(ctx, addr, tlsConf, quicConf)
	if err != nil {
		return nil, nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, nil, err
	}
	remoteHello, err := exchangeHello(stream, cfg, keys)
	if err != nil {
		return nil, nil, closeConn(conn, stream, err)
	}
	if peerID != "" && remoteHello.PeerID != peerID {
		return nil, nil, closeConn(conn, stream, quic.ErrPeerIDMismatch)
	}
	return conn, stream, nil
}

func normalizeAddr(addr string) (string, error) {
	addr = stripScheme(addr)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return "", quic.ErrAddrInvalid
	}
	return addr, nil
}

func clientConfigs() (quic.KeyMaterial, *tls.Config, *quicgo.Config, error) {
	keys, err := quic.LoadOrCreateKeyMaterial("")
	if err != nil {
		return quic.KeyMaterial{}, nil, nil, err
	}
	tlsConf := &tls.Config{
		Certificates:       []tls.Certificate{keys.TLSCert},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	quicConf := &quicgo.Config{
		HandshakeIdleTimeout: 10 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
		KeepAlivePeriod:      10 * time.Second,
	}
	return keys, tlsConf, quicConf, nil
}

func closeConn(conn quicgo.Connection, stream quicgo.Stream, err error) error {
	_ = stream.Close()
	_ = conn.CloseWithError(0, "")
	return err
}

func exchangeHello(stream quicgo.Stream, cfg config.Config, keys quic.KeyMaterial) (quic.Hello, error) {
	localPeerID, err := ids.NewPeerID(keys.PublicKey)
	if err != nil {
		return quic.Hello{}, err
	}
	localHello := quic.Hello{
		V:             quic.HelloVersion,
		PeerID:        localPeerID,
		TSMs:          timeutil.NowUnixMs(),
		Nonce:         make([]byte, 16),
		PowDifficulty: cfg.Pow.DefaultDifficulty,
		MaxMsgBytes:   cfg.Limits.MaxMsgBytes,
	}
	localBytes, err := quic.EncodeHello(localHello)
	if err != nil {
		return quic.Hello{}, err
	}
	if err := quic.WriteFrame(stream, localBytes); err != nil {
		return quic.Hello{}, err
	}
	remoteBytes, err := quic.ReadFrame(stream, cfg.Limits.MaxMsgBytes)
	if err != nil {
		return quic.Hello{}, err
	}
	remoteHello, err := quic.DecodeHello(remoteBytes)
	if err != nil {
		return quic.Hello{}, err
	}
	if err := quic.ValidateHello(timeutil.NowUnixMs(), remoteHello); err != nil {
		return quic.Hello{}, err
	}
	return remoteHello, nil
}

func readAckAndResult(stream quicgo.Stream, cfg config.Config, ctx context.Context) (*ackStatus, *envelope.Envelope, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetReadDeadline(deadline)
	}
	var ack *ackStatus
	var result *envelope.Envelope
	for {
		frame, err := quic.ReadFrame(stream, cfg.Limits.MaxMsgBytes)
		if err != nil {
			return ack, result, err
		}
		env, err := envelope.DecodeEnvelope(frame)
		if err != nil {
			continue
		}
		if env.Type == "ack.v1" {
			ackPayload := struct {
				V         uint64 `cbor:"v"`
				MsgID     string `cbor:"msg_id"`
				Status    string `cbor:"status"`
				ErrorCode string `cbor:"error_code,omitempty"`
			}{}
			if err := codec.Unmarshal(env.Payload, &ackPayload); err == nil {
				ack = &ackStatus{Status: ackPayload.Status, ErrorCode: ackPayload.ErrorCode}
			}
			continue
		}
		if env.Type == tasks.AcceptType || env.Type == tasks.ProgressType {
			continue
		}
		if env.Type == tasks.ResultType || env.Type == tasks.FailType || env.Type == nodefetch.ResponseType {
			result = env
			return ack, result, nil
		}
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
	return os.ReadFile(path)
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

func stripScheme(addr string) string {
	const prefix = "quic://"
	if strings.HasPrefix(addr, prefix) {
		return strings.TrimPrefix(addr, prefix)
	}
	return addr
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
