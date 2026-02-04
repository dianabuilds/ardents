package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dianabuilds/ardents/internal/core/app/runtime"
	"github.com/dianabuilds/ardents/internal/core/domain/contentnode"
	"github.com/dianabuilds/ardents/internal/core/transport/cliutil"
	"github.com/dianabuilds/ardents/internal/shared/codec"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
	"github.com/dianabuilds/ardents/internal/shared/webtypes"
)

type NodeView struct {
	NodeID       string             `json:"node_id"`
	Type         string             `json:"type"`
	Owner        string             `json:"owner"`
	CreatedAtMs  int64              `json:"created_at_ms"`
	Policy       map[string]any     `json:"policy,omitempty"`
	Links        []contentnode.Link `json:"links,omitempty"`
	Encrypted    bool               `json:"encrypted"`
	VerifyStatus string             `json:"verify_status"`
	Prev         []string           `json:"prev,omitempty"`
	Supersedes   []string           `json:"supersedes,omitempty"`
	History      []NodeView         `json:"history,omitempty"`
	Body         any                `json:"body,omitempty"`
	Pretty       any                `json:"pretty,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "get":
		getCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage: node get --target <ufa> [--decrypt] [--show-body] [--pretty] [--history-depth N] [--peer-id <peer_id> --addr <host:port>]")
	fmt.Println("  --target <ufa>             alias|node_id (UFA)")
}

func getCmd(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	target := fs.String("target", "", "node ufa: alias or node_id (required)")
	decrypt := fs.Bool("decrypt", false, "attempt decrypt if encrypted")
	showBody := fs.Bool("show-body", false, "include decoded body in output")
	pretty := fs.Bool("pretty", false, "pretty output for known node types")
	historyDepth := fs.Int("history-depth", 0, "follow prev/supersedes links (depth)")
	peerID := fs.String("peer-id", "", "peer id for direct fetch")
	addr := fs.String("addr", "", "peer address host:port (required with --peer-id)")
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	rawTarget := *target
	if rawTarget == "" {
		fatal(errors.New("ERR_UFA_REQUIRED"))
	}
	cfg, err := cliutil.LoadConfig(*home, *cfgPath)
	if err != nil {
		fatal(err)
	}
	book, err := cliutil.LoadAddressBook(*home)
	if err != nil {
		fatal(err)
	}
	nodeID, err := cliutil.ResolveNodeID(rawTarget, book, timeutil.NowUnixMs())
	if err != nil {
		fatal(err)
	}
	rt := runtime.New(cfg)
	var bytes []byte
	if *peerID != "" || *addr != "" {
		if *peerID == "" || *addr == "" {
			fatal(errors.New("ERR_CLI_INVALID_ARGS"))
		}
		bytes, err = rt.FetchNodeFromPeer(context.Background(), *addr, *peerID, nodeID)
	} else {
		bytes, err = rt.FetchNode(context.Background(), nodeID)
	}
	if err != nil {
		fatal(err)
	}
	view, err := buildView(bytes, nodeID, *decrypt, *showBody, *pretty, rt)
	if err != nil {
		fatal(err)
	}
	if *historyDepth > 0 {
		view.History = followHistory(context.Background(), rt, view, *historyDepth, *decrypt, *showBody, *pretty)
	}
	out, _ := json.MarshalIndent(view, "", "  ")
	fmt.Println(string(out))
}

func buildView(nodeBytes []byte, nodeID string, decrypt bool, showBody bool, pretty bool, rt *runtime.Runtime) (NodeView, error) {
	verifyErr := contentnode.VerifyBytes(nodeBytes, nodeID)
	verifyStatus := "ok"
	if verifyErr != nil {
		verifyStatus = "invalid"
	}
	var n contentnode.Node
	if err := contentnode.Decode(nodeBytes, &n); err != nil {
		return NodeView{}, err
	}
	view := NodeView{
		NodeID:       nodeID,
		Type:         n.Type,
		Owner:        n.Owner,
		CreatedAtMs:  n.CreatedAtMs,
		Policy:       n.Policy,
		Links:        n.Links,
		VerifyStatus: verifyStatus,
	}
	if n.Type == "enc.node.v1" {
		view.Encrypted = true
		if decrypt && rt.IdentityID() != "" && rt.IdentityPrivateKey() != nil {
			payload, err := contentnode.DecryptNode(n, rt.IdentityID(), rt.IdentityPrivateKey())
			if err == nil {
				view.Type = payload.Type
				view.Links = payload.Links
				view.Encrypted = false
				if showBody {
					view.Body = payload.Body
				}
				if pretty {
					view.Pretty = prettyBody(payload.Type, payload.Body)
				}
			}
		}
	} else {
		if showBody {
			view.Body = n.Body
		}
		if pretty {
			view.Pretty = prettyBody(n.Type, n.Body)
		}
	}
	for _, l := range view.Links {
		if l.Rel == "prev" {
			view.Prev = append(view.Prev, l.NodeID)
		}
		if l.Rel == "supersedes" {
			view.Supersedes = append(view.Supersedes, l.NodeID)
		}
	}
	return view, nil
}

func followHistory(ctx context.Context, rt *runtime.Runtime, root NodeView, depth int, decrypt bool, showBody bool, pretty bool) []NodeView {
	if depth <= 0 {
		return nil
	}
	seen := map[string]bool{root.NodeID: true}
	queue := make([]string, 0, len(root.Prev)+len(root.Supersedes))
	queue = append(queue, root.Prev...)
	queue = append(queue, root.Supersedes...)
	out := make([]NodeView, 0)
	for depth > 0 && len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next == "" || seen[next] {
			continue
		}
		seen[next] = true
		bytes, err := rt.FetchNode(ctx, next)
		if err != nil {
			continue
		}
		view, err := buildView(bytes, next, decrypt, showBody, pretty, rt)
		if err != nil {
			continue
		}
		out = append(out, view)
		queue = append(queue, view.Prev...)
		queue = append(queue, view.Supersedes...)
		depth--
	}
	return out
}

func prettyBody(nodeType string, body any) any {
	switch nodeType {
	case "web.response.v1":
		var resp webtypes.ResponseV1
		b, err := codec.Marshal(body)
		if err != nil {
			return nil
		}
		if err := codec.Unmarshal(b, &resp); err != nil {
			return nil
		}
		return map[string]any{
			"status":  resp.Status,
			"headers": resp.Headers,
			"body":    string(resp.Body),
		}
	default:
		return nil
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
