package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/dianabuilds/ardents/internal/core/infra/config"
	"github.com/dianabuilds/ardents/internal/core/transport/quic"
	"github.com/dianabuilds/ardents/internal/shared/appdirs"
)

type pcapRecord struct {
	Dir             string `json:"dir"`
	TSMs            int64  `json:"ts_ms"`
	PeerIDRemote    string `json:"peer_id_remote,omitempty"`
	EnvelopeCBORB64 string `json:"envelope_cbor_b64"`
}

func main() {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	pcapPath := fs.String("pcap", "", "pcap jsonl path (default: XDG/ARDENTS_HOME)")
	addr := fs.String("addr", "", "quic://host:port (required)")
	expectedPeerID := fs.String("peer", "", "expected remote peer_id (optional)")
	allowNetwork := fs.Bool("allow-network", false, "allow non-loopback replay")
	noDelay := fs.Bool("no-delay", false, "replay without delays")
	onlyDir := fs.String("dir", "out", "filter by dir (out|in|any)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fatal(err)
	}
	if *addr == "" {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	if !*allowNetwork && !isLoopbackAddr(*addr) {
		fatal(errors.New("ERR_CLI_INVALID_ARGS"))
	}
	if *pcapPath == "" {
		if *home != "" {
			_ = os.Setenv(appdirs.EnvHome, *home)
		}
		dirs, err := appdirs.Resolve(*home)
		if err != nil {
			fatal(err)
		}
		*pcapPath = dirs.PcapPath()
	}

	cfg := config.Default()
	dialer, err := quic.NewDialer(cfg)
	if err != nil {
		fatal(err)
	}
	file, err := os.Open(*pcapPath)
	if err != nil {
		fatal(err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	var prevTS int64
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec pcapRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			fatal(err)
		}
		if rec.Dir == "" || rec.EnvelopeCBORB64 == "" {
			continue
		}
		if *onlyDir != "any" && rec.Dir != *onlyDir {
			continue
		}
		if !*noDelay && prevTS > 0 && rec.TSMs > prevTS {
			time.Sleep(time.Duration(rec.TSMs-prevTS) * time.Millisecond)
		}
		prevTS = rec.TSMs
		data, err := base64.StdEncoding.DecodeString(rec.EnvelopeCBORB64)
		if err != nil {
			fatal(err)
		}
		if _, err := dialer.SendEnvelope(context.Background(), stripScheme(*addr), *expectedPeerID, data, cfg.Limits.MaxMsgBytes); err != nil {
			fatal(err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		fatal(err)
	}
	fmt.Println("replayed:", count)
}

func stripScheme(addr string) string {
	const prefix = "quic://"
	if len(addr) >= len(prefix) && addr[:len(prefix)] == prefix {
		return addr[len(prefix):]
	}
	return addr
}

func isLoopbackAddr(addr string) bool {
	host := stripScheme(addr)
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
