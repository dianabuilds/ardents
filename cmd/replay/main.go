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

type replayOptions struct {
	home         string
	pcapPath     string
	addr         string
	expectedPeer string
	allowNetwork bool
	noDelay      bool
	onlyDir      string
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	cfg := config.Default()
	dialer, err := quic.NewDialer(cfg)
	if err != nil {
		fatal(err)
	}
	file, err := os.Open(opts.pcapPath)
	if err != nil {
		fatal(err)
	}
	defer func() {
		_ = file.Close()
	}()

	count, err := replayFile(file, dialer, cfg, opts)
	if err != nil {
		fatal(err)
	}
	fmt.Println("replayed:", count)
}

func parseOptions(args []string) (replayOptions, error) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	home := fs.String("home", "", "portable mode root (also Env: ARDENTS_HOME)")
	pcapPath := fs.String("pcap", "", "pcap jsonl path (default: XDG/ARDENTS_HOME)")
	addr := fs.String("addr", "", "quic://host:port (required)")
	expectedPeerID := fs.String("peer", "", "expected remote peer_id (optional)")
	allowNetwork := fs.Bool("allow-network", false, "allow non-loopback replay")
	noDelay := fs.Bool("no-delay", false, "replay without delays")
	onlyDir := fs.String("dir", "out", "filter by dir (out|in|any)")
	if err := fs.Parse(args); err != nil {
		return replayOptions{}, err
	}
	opts := replayOptions{
		home:         *home,
		pcapPath:     *pcapPath,
		addr:         *addr,
		expectedPeer: *expectedPeerID,
		allowNetwork: *allowNetwork,
		noDelay:      *noDelay,
		onlyDir:      *onlyDir,
	}
	if opts.addr == "" {
		return replayOptions{}, errors.New("ERR_CLI_INVALID_ARGS")
	}
	if !opts.allowNetwork && !isLoopbackAddr(opts.addr) {
		return replayOptions{}, errors.New("ERR_CLI_INVALID_ARGS")
	}
	resolved, err := resolvePcapPath(opts)
	if err != nil {
		return replayOptions{}, err
	}
	return resolved, nil
}

func resolvePcapPath(opts replayOptions) (replayOptions, error) {
	if opts.pcapPath != "" {
		return opts, nil
	}
	if opts.home != "" {
		_ = os.Setenv(appdirs.EnvHome, opts.home)
	}
	dirs, err := appdirs.Resolve(opts.home)
	if err != nil {
		return replayOptions{}, err
	}
	opts.pcapPath = dirs.PcapPath()
	return opts, nil
}

func replayFile(file *os.File, dialer *quic.Dialer, cfg config.Config, opts replayOptions) (int, error) {
	scanner := bufio.NewScanner(file)
	var prevTS int64
	count := 0
	for scanner.Scan() {
		rec, ok, err := decodePcapRecord(scanner.Bytes())
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		if !shouldReplayDir(rec.Dir, opts.onlyDir) {
			continue
		}
		if !opts.noDelay && prevTS > 0 && rec.TSMs > prevTS {
			time.Sleep(time.Duration(rec.TSMs-prevTS) * time.Millisecond)
		}
		prevTS = rec.TSMs
		if err := sendReplayEnvelope(dialer, cfg, opts, rec.EnvelopeCBORB64); err != nil {
			return 0, err
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func decodePcapRecord(line []byte) (pcapRecord, bool, error) {
	var rec pcapRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return pcapRecord{}, false, err
	}
	if rec.Dir == "" || rec.EnvelopeCBORB64 == "" {
		return pcapRecord{}, false, nil
	}
	return rec, true, nil
}

func sendReplayEnvelope(dialer *quic.Dialer, cfg config.Config, opts replayOptions, envelopeB64 string) error {
	data, err := base64.StdEncoding.DecodeString(envelopeB64)
	if err != nil {
		return err
	}
	_, err = dialer.SendEnvelope(context.Background(), stripScheme(opts.addr), opts.expectedPeer, data, cfg.Limits.MaxMsgBytes)
	return err
}

func shouldReplayDir(dir string, filter string) bool {
	if filter == "any" {
		return true
	}
	return dir == filter
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
