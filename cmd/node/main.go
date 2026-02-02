package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/contentnode"
	"github.com/dianabuilds/ardents/internal/runtime"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
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
	fmt.Println("usage: node get --id <cid> [--decrypt] [--history-depth N]")
}

func getCmd(args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	id := fs.String("id", "", "node cid (required)")
	decrypt := fs.Bool("decrypt", false, "attempt decrypt if encrypted")
	historyDepth := fs.Int("history-depth", 0, "follow prev/supersedes links (depth)")
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	cfgPath := fs.String("config", "", "path to config file (default: XDG/ARDENTS_HOME)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	if *id == "" {
		fatal(errors.New("missing --id"))
	}
	if *home != "" {
		_ = os.Setenv(appdirs.EnvHome, *home)
	}
	dirs, err := appdirs.Resolve(*home)
	if err != nil {
		fatal(err)
	}
	if *cfgPath == "" {
		*cfgPath = dirs.ConfigPath()
	}
	cfg, err := loadOrInitConfig(*cfgPath)
	if err != nil {
		fatal(err)
	}
	rt := runtime.New(cfg)
	bytes, err := rt.FetchNode(context.Background(), *id)
	if err != nil {
		fatal(err)
	}
	view, err := buildView(bytes, *id, *decrypt, rt)
	if err != nil {
		fatal(err)
	}
	if *historyDepth > 0 {
		view.History = followHistory(context.Background(), rt, view, *historyDepth, *decrypt)
	}
	out, _ := json.MarshalIndent(view, "", "  ")
	fmt.Println(string(out))
}

func buildView(nodeBytes []byte, nodeID string, decrypt bool, rt *runtime.Runtime) (NodeView, error) {
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
			}
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

func followHistory(ctx context.Context, rt *runtime.Runtime, root NodeView, depth int, decrypt bool) []NodeView {
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
		view, err := buildView(bytes, next, decrypt, rt)
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

func loadOrInitConfig(path string) (config.Config, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, nil
	}
	cfg = config.Default()
	if err := config.Save(path, cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
