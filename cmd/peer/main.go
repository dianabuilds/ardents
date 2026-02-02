package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/dianabuilds/ardents/internal/addressbook"
	"github.com/dianabuilds/ardents/internal/config"
	"github.com/dianabuilds/ardents/internal/contentnode"
	"github.com/dianabuilds/ardents/internal/runtime"
	"github.com/dianabuilds/ardents/internal/shared/ack"
	"github.com/dianabuilds/ardents/internal/shared/envelope"
	"github.com/dianabuilds/ardents/internal/shared/identity"
	"github.com/dianabuilds/ardents/internal/shared/ids"
	"github.com/dianabuilds/ardents/internal/shared/perm"
	"github.com/dianabuilds/ardents/internal/shared/timeutil"
)

type Status struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons,omitempty"`
	TSMs    int64    `json:"ts_ms"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		startCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "send":
		sendCmd(os.Args[2:])
	case "addressbook":
		addressBookCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage: peer <start|status|send> [flags]")
	fmt.Println("       peer addressbook <list|add> [flags]")
}

func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultConfigPath, "path to config file")
	pcap := fs.Bool("pcap", false, "enable packet capture (run/pcap.jsonl)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	cfg, err := loadOrInitConfig(*cfgPath)
	if err != nil {
		fatal(err)
	}
	if *pcap {
		cfg.Observability.PcapEnabled = true
	}

	if err := rotateGatewayToken("run/peer.token"); err != nil {
		fatal(err)
	}
	rt := runtime.New(cfg)
	if err := rt.Start(context.Background()); err != nil {
		fatal(err)
	}
	status := Status{State: string(rt.NetState()), Reasons: rt.NetReasons(), TSMs: timeutil.NowUnixMs()}
	if err := writeStatus(status); err != nil {
		fatal(err)
	}
	fmt.Println("peer started")
	if addr := rt.QUICAddr(); addr != "" {
		fmt.Println("quic:", addr)
	}
	waitForSignal()
}

func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultConfigPath, "path to config file")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	st, err := readStatus()
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
	cfg, err := loadOrInitConfig(*cfgPath)
	if err != nil {
		fatal(err)
	}
	if cfg.Observability.HealthAddr != "" {
		url := "http://" + cfg.Observability.HealthAddr + "/healthz"
		if h, err := http.Get(url); err == nil {
			defer func() {
				_ = h.Body.Close()
			}()
			body, err := io.ReadAll(h.Body)
			if err != nil {
				fatal(err)
			}
			fmt.Println("healthz:", string(body))
		}
	}
}

func sendCmd(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	to := fs.String("to", "", "alias|peer_id|service_id")
	text := fs.String("text", "", "message text")
	addr := fs.String("addr", "", "quic://host:port (required for now)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	if *to == "" || *text == "" || *addr == "" {
		fatal(errors.New("missing --to or --text or --addr"))
	}

	cfg, err := loadOrInitConfig(config.DefaultConfigPath)
	if err != nil {
		fatal(err)
	}
	rt := runtime.New(cfg)
	peerID, err := resolveTargetPeerID(*to)
	if err != nil {
		fatal(err)
	}
	printIndicators(rt, peerID)
	ackBytes, err := rt.SendChat(context.Background(), *addr, peerID, *text)
	if err != nil {
		fatal(err)
	}
	ackEnv, err := envelope.DecodeEnvelope(ackBytes)
	if err != nil {
		fatal(err)
	}
	ackPayload, err := ack.Decode(ackEnv.Payload)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("ack: status=%s error=%s\n", ackPayload.Status, ackPayload.ErrorCode)
	if ackPayload.Status == "REJECTED" {
		fmt.Println("error:", ackPayload.ErrorCode)
	}
}

func resolveTargetPeerID(to string) (string, error) {
	if to == "" {
		return "", errors.New("missing --to")
	}
	if err := ids.ValidatePeerID(to); err == nil {
		return to, nil
	}
	book, err := addressbook.LoadOrInit("")
	if err != nil {
		return "", err
	}
	entry, ok, err := book.ResolveAlias(to, timeutil.NowUnixMs())
	if err != nil {
		if errors.Is(err, addressbook.ErrAliasConflict) {
			return "", addressbook.ErrAliasConflict
		}
		return "", err
	}
	if !ok {
		return "", errors.New("ERR_ALIAS_NOT_FOUND")
	}
	if entry.TargetType != "peer" {
		return "", errors.New("ERR_ALIAS_TARGET_NOT_PEER")
	}
	return entry.TargetID, nil
}

func addressBookCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		addressBookList(args[1:])
	case "add":
		addressBookAdd(args[1:])
	case "export":
		addressBookExport(args[1:])
	case "import":
		addressBookImport(args[1:])
	default:
		fmt.Println("usage: peer addressbook <list|add|export|import> [flags]")
		os.Exit(2)
	}
}

func printIndicators(rt *runtime.Runtime, toID string) {
	trusted := rt.IdentityID() != "" && rt.IdentityID() == toID
	if trusted {
		fmt.Println("trust: trusted")
	} else {
		fmt.Println("trust: untrusted")
	}
	fmt.Println("pow: required for untrusted")
	fmt.Println("net:", rt.NetState())
}

func addressBookList(args []string) {
	fs := flag.NewFlagSet("addressbook list", flag.ExitOnError)
	path := fs.String("path", "", "path to addressbook.json")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func addressBookAdd(args []string) {
	fs := flag.NewFlagSet("addressbook add", flag.ExitOnError)
	path := fs.String("path", "", "path to addressbook.json")
	alias := fs.String("alias", "", "alias (required)")
	identityID := fs.String("identity", "", "identity_id (did:key:...) required")
	trust := fs.String("trust", "trusted", "trusted|untrusted")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	if *alias == "" || *identityID == "" {
		fatal(errors.New("missing --alias or --identity"))
	}
	if err := ids.ValidateAlias(*alias); err != nil {
		fatal(err)
	}
	if err := ids.ValidateIdentityID(*identityID); err != nil {
		fatal(err)
	}
	if *trust != "trusted" && *trust != "untrusted" {
		fatal(errors.New("trust must be trusted|untrusted"))
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	entry := addressbook.Entry{
		Alias:       *alias,
		TargetType:  "identity",
		TargetID:    *identityID,
		Source:      "self",
		Trust:       *trust,
		Note:        *note,
		CreatedAtMs: timeutil.NowUnixMs(),
	}
	book.Entries = append(book.Entries, entry)
	book.UpdatedAtMs = timeutil.NowUnixMs()
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook entry added")
}

func addressBookExport(args []string) {
	fs := flag.NewFlagSet("addressbook export", flag.ExitOnError)
	path := fs.String("path", "", "path to addressbook.json")
	out := fs.String("out", "addressbook.bundle.cbor", "output file")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	id, err := identity.LoadOrCreate("")
	if err != nil {
		fatal(err)
	}
	node, err := book.ExportBundle(id)
	if err != nil {
		fatal(err)
	}
	data, err := contentnode.Encode(node)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle exported:", *out)
}

func addressBookImport(args []string) {
	fs := flag.NewFlagSet("addressbook import", flag.ExitOnError)
	path := fs.String("path", "", "path to addressbook.json")
	in := fs.String("in", "", "input bundle file (required)")
	if err := fs.Parse(args); err != nil {
		fatal(err)
	}

	if *in == "" {
		fatal(errors.New("missing --in"))
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	var node contentnode.Node
	if err := contentnode.Decode(data, &node); err != nil {
		fatal(err)
	}
	book, err := addressbook.LoadOrInit(*path)
	if err != nil {
		fatal(err)
	}
	book, err = book.ImportBundle(node, timeutil.NowUnixMs())
	if err != nil {
		fatal(err)
	}
	if err := addressbook.Save(*path, book); err != nil {
		fatal(err)
	}
	fmt.Println("addressbook bundle imported")
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

func writeStatus(st Status) error {
	if err := os.MkdirAll("run", 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("run", "status.json"), b, 0o644)
}

func readStatus() (Status, error) {
	b, err := os.ReadFile(filepath.Join("run", "status.json"))
	if err != nil {
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		return Status{}, err
	}
	return st, nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}

func rotateGatewayToken(path string) error {
	f, err := perm.OpenOwnerOnly(path)
	if err != nil {
		return errors.New("ERR_GATEWAY_UNAUTHORIZED")
	}
	defer func() {
		_ = f.Close()
	}()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.StdEncoding.EncodeToString(raw)
	if _, err := f.WriteString(token); err != nil {
		return err
	}
	return nil
}
