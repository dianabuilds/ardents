package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/perm"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

const (
	defaultServiceName = "web.request.v1"
	defaultTimeoutMs   = 10_000
	defaultMaxBody     = 512 * 1024
)

type ipcMessage struct {
	Type            string          `json:"type"`
	Token           string          `json:"token,omitempty"`
	ServiceName     string          `json:"service_name,omitempty"`
	JobTypes        []string        `json:"job_types,omitempty"`
	ServiceID       string          `json:"service_id,omitempty"`
	TaskID          string          `json:"task_id,omitempty"`
	ClientRequestID string          `json:"client_request_id,omitempty"`
	JobType         string          `json:"job_type,omitempty"`
	Input           json.RawMessage `json:"input,omitempty"`
	TSMs            int64           `json:"ts_ms,omitempty"`
	ResultNode      string          `json:"result_node,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
}

type webRequestInput struct {
	V       uint64            `json:"v"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
	Policy  map[string]any    `json:"policy,omitempty"`
}

type webResponseBody struct {
	V       uint64            `cbor:"v"`
	TaskID  string            `cbor:"task_id"`
	Status  uint16            `cbor:"status"`
	Headers map[string]string `cbor:"headers,omitempty"`
	Body    []byte            `cbor:"body,omitempty"`
}

type ipcConfig struct {
	home        string
	serviceName string
	jobType     string
	upstream    string
	timeoutMs   int
	maxBody     int
}

type upstreamResponse struct {
	status  uint16
	headers map[string]string
	body    []byte
}

func main() {
	cfg := parseFlags()
	baseURL, err := parseUpstream(cfg.upstream)
	if err != nil {
		fatal(err)
	}
	dirs, err := appdirs.Resolve(cfg.home)
	if err != nil {
		fatal(err)
	}
	token, err := readToken(dirs)
	if err != nil {
		fatal(err)
	}
	id, err := identity.LoadOrCreate(dirs.IdentityDir())
	if err != nil {
		fatal(err)
	}
	conn, err := ipcDial(dirs)
	if err != nil {
		fatal(err)
	}
	defer func() {
		_ = conn.Close()
	}()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	if err := registerIPC(enc, dec, token, cfg.serviceName, cfg.jobType); err != nil {
		fatal(err)
	}

	client := &http.Client{
		Timeout: time.Duration(cfg.timeoutMs) * time.Millisecond,
	}
	log.Printf("integration ipc registered: service=%s job=%s upstream=%s", cfg.serviceName, cfg.jobType, baseURL.String())
	for {
		var msg ipcMessage
		if err := dec.Decode(&msg); err != nil {
			fatal(err)
		}
		if msg.Type != "task" {
			continue
		}
		res := handleTask(msg, cfg.jobType, baseURL, client, id, cfg.maxBody)
		if err := enc.Encode(res); err != nil {
			fatal(err)
		}
	}
}

func handleTask(msg ipcMessage, expectedJob string, baseURL *url.URL, client *http.Client, id identity.Identity, maxBody int) ipcMessage {
	if err := validateTask(msg, expectedJob); err != nil {
		return ipcMessage{Type: "result", TaskID: msg.TaskID, ErrorCode: err.Error()}
	}
	input, err := decodeWebInput(msg.Input)
	if err != nil {
		return ipcMessage{Type: "result", TaskID: msg.TaskID, ErrorCode: err.Error()}
	}
	req, err := buildUpstreamRequest(baseURL, input, maxBody)
	if err != nil {
		return ipcMessage{Type: "result", TaskID: msg.TaskID, ErrorCode: err.Error()}
	}
	resp, err := doUpstreamRequest(client, req, maxBody)
	if err != nil {
		return ipcMessage{Type: "result", TaskID: msg.TaskID, ErrorCode: err.Error()}
	}
	nodeBytes, err := buildResponseNodeBytes(msg.TaskID, resp, input.Policy, id)
	if err != nil {
		return ipcMessage{Type: "result", TaskID: msg.TaskID, ErrorCode: err.Error()}
	}
	return ipcMessage{Type: "result", TaskID: msg.TaskID, ResultNode: base64.StdEncoding.EncodeToString(nodeBytes)}
}

func buildResponseNode(taskID string, status uint16, headers map[string]string, body []byte, policy map[string]any, id identity.Identity) (contentnode.Node, error) {
	respBody := webResponseBody{
		V:       1,
		TaskID:  taskID,
		Status:  status,
		Headers: headers,
		Body:    body,
	}
	node := contentnode.Node{
		V:           1,
		Type:        "web.response.v1",
		CreatedAtMs: timeutil.NowUnixMs(),
		Owner:       id.ID,
		Links:       []contentnode.Link{},
		Body:        respBody,
		Policy:      policyOrDefault(policy),
	}
	if err := contentnode.Sign(&node, id.PrivateKey); err != nil {
		return contentnode.Node{}, err
	}
	return node, nil
}

func parseFlags() ipcConfig {
	fs := flag.NewFlagSet("integration-ipc", flag.ExitOnError)
	cfg := ipcConfig{}
	fs.StringVar(&cfg.home, "home", "", "portable mode root (also Env: ARDENTS_HOME)")
	fs.StringVar(&cfg.serviceName, "service", defaultServiceName, "service_name to register")
	fs.StringVar(&cfg.jobType, "job", defaultServiceName, "job_type to handle")
	fs.StringVar(&cfg.upstream, "upstream", "", "http(s) upstream base url (example: http://127.0.0.1:8080)")
	fs.IntVar(&cfg.timeoutMs, "timeout-ms", defaultTimeoutMs, "upstream request timeout (ms)")
	fs.IntVar(&cfg.maxBody, "max-body-bytes", defaultMaxBody, "max upstream response body size")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fatal(err)
	}
	if cfg.upstream == "" {
		fatal(errors.New("ERR_UPSTREAM_REQUIRED"))
	}
	if cfg.jobType == "" || cfg.serviceName == "" {
		fatal(errors.New("ERR_SERVICE_INVALID"))
	}
	if cfg.timeoutMs <= 0 {
		cfg.timeoutMs = defaultTimeoutMs
	}
	if cfg.maxBody <= 0 {
		cfg.maxBody = defaultMaxBody
	}
	return cfg
}

func readToken(dirs appdirs.Dirs) (string, error) {
	if err := perm.EnsureOwnerOnly(dirs.GatewayTokenPath()); err != nil {
		return "", err
	}
	tokenBytes, err := os.ReadFile(dirs.GatewayTokenPath())
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", errors.New("ERR_IPC_TOKEN_EMPTY")
	}
	return token, nil
}

func registerIPC(enc *json.Encoder, dec *json.Decoder, token string, serviceName string, jobType string) error {
	if err := enc.Encode(ipcMessage{Type: "auth", Token: token}); err != nil {
		return err
	}
	if err := enc.Encode(ipcMessage{Type: "register", ServiceName: serviceName, JobTypes: []string{jobType}}); err != nil {
		return err
	}
	var reg ipcMessage
	if err := dec.Decode(&reg); err != nil || reg.Type != "registered" {
		return errors.New("ERR_IPC_REGISTER_FAILED")
	}
	return nil
}

func validateTask(msg ipcMessage, expectedJob string) error {
	if msg.TaskID == "" || msg.JobType != expectedJob {
		return errors.New("ERR_INTEGRATION_BAD_TASK")
	}
	return nil
}

func decodeWebInput(raw json.RawMessage) (webRequestInput, error) {
	var input webRequestInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return webRequestInput{}, errors.New("ERR_WEB_INPUT_INVALID")
	}
	if input.V != 1 {
		return webRequestInput{}, errors.New("ERR_WEB_INPUT_INVALID")
	}
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	if input.Path == "" {
		input.Path = "/"
	}
	return input, nil
}

func buildUpstreamRequest(baseURL *url.URL, input webRequestInput, maxBody int) (*http.Request, error) {
	rel, err := parseRelative(input.Path)
	if err != nil {
		return nil, errors.New("ERR_WEB_INPUT_INVALID")
	}
	maxBody = clampBodyLimit(maxBody)
	if len(input.Body) > maxBody {
		return nil, errors.New("ERR_WEB_INPUT_TOO_LARGE")
	}
	reqURL := baseURL.ResolveReference(rel)
	req, err := http.NewRequestWithContext(context.Background(), input.Method, reqURL.String(), bytes.NewReader(input.Body))
	if err != nil {
		return nil, errors.New("ERR_WEB_UPSTREAM_FAILED")
	}
	applyHeaders(req, input.Headers)
	return req, nil
}

func doUpstreamRequest(client *http.Client, req *http.Request, maxBody int) (upstreamResponse, error) {
	resp, err := client.Do(req)
	if err != nil {
		return upstreamResponse{}, errors.New("ERR_WEB_UPSTREAM_FAILED")
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := readLimited(resp.Body, clampBodyLimit(maxBody))
	if err != nil {
		return upstreamResponse{}, errors.New("ERR_WEB_UPSTREAM_FAILED")
	}
	return upstreamResponse{
		status:  uint16(resp.StatusCode),
		headers: flattenHeaders(resp.Header),
		body:    body,
	}, nil
}

func buildResponseNodeBytes(taskID string, resp upstreamResponse, policy map[string]any, id identity.Identity) ([]byte, error) {
	node, err := buildResponseNode(taskID, resp.status, resp.headers, resp.body, policy, id)
	if err != nil {
		return nil, errors.New("ERR_WEB_RESPONSE_INVALID")
	}
	nodeBytes, err := contentnode.Encode(node)
	if err != nil {
		return nil, errors.New("ERR_WEB_RESPONSE_INVALID")
	}
	if err := contentnode.VerifyBytes(nodeBytes, ""); err != nil {
		return nil, errors.New("ERR_WEB_RESPONSE_INVALID")
	}
	return nodeBytes, nil
}

func clampBodyLimit(maxBody int) int {
	if maxBody > contentnode.MaxNodeBytes {
		return contentnode.MaxNodeBytes
	}
	return maxBody
}

func policyOrDefault(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{
			"v":          uint64(1),
			"visibility": "public",
		}
	}
	return input
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		if k == "" || v == "" {
			continue
		}
		if isHopHeader(k) || http.CanonicalHeaderKey(k) == "Host" {
			continue
		}
		req.Header.Set(k, v)
	}
}

func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range h {
		if isHopHeader(k) {
			continue
		}
		if len(v) == 0 {
			continue
		}
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func isHopHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Connection", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Keep-Alive":
		return true
	default:
		return false
	}
}

func parseUpstream(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("ERR_UPSTREAM_INVALID")
	}
	if u.Host == "" {
		return nil, errors.New("ERR_UPSTREAM_INVALID")
	}
	if !isLoopbackHost(u.Host) {
		return nil, errors.New("ERR_UPSTREAM_INVALID")
	}
	return u, nil
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func parseRelative(path string) (*url.URL, error) {
	if strings.Contains(path, "://") {
		return nil, errors.New("ERR_PATH_INVALID")
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	if u.IsAbs() || u.Host != "" {
		return nil, errors.New("ERR_PATH_INVALID")
	}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	return u, nil
}

func readLimited(r io.Reader, max int) ([]byte, error) {
	if max <= 0 {
		return io.ReadAll(r)
	}
	return io.ReadAll(io.LimitReader(r, int64(max)))
}

func fatal(err error) {
	if err == nil {
		return
	}
	log.Fatal(err)
}
