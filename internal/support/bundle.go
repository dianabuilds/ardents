package support

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

const (
	DefaultTailLines = 2000
	MaxTailBytes     = 1 << 20 // 1 MiB
)

var ErrBadRequest = errors.New("ERR_SUPPORT_BAD_REQUEST")

type BundleOptions struct {
	OutPath            string
	ConfigPath         string
	StatusPath         string
	AddressBookPath    string
	LogFilePath        string
	PcapPath           string
	TailLines          int
	IncludeAddressBook bool
}

func WriteBundle(opts BundleOptions) (string, error) {
	if opts.OutPath == "" {
		return "", ErrBadRequest
	}
	if opts.TailLines <= 0 {
		opts.TailLines = DefaultTailLines
	}
	outPath := opts.OutPath
	if filepath.Ext(outPath) != ".zip" {
		outPath += ".zip"
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	now := time.Now().UTC()
	_ = addJSON(zw, "meta/created.json", map[string]any{
		"ts":         now.Format(time.RFC3339Nano),
		"ts_ms":      now.UnixMilli(),
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"build_info": buildInfo(),
	})
	_ = addJSON(zw, "meta/paths.json", map[string]any{
		"config_path":      opts.ConfigPath,
		"status_path":      opts.StatusPath,
		"addressbook_path": opts.AddressBookPath,
		"log_file_path":    opts.LogFilePath,
		"pcap_path":        opts.PcapPath,
		"bundle_path":      outPath,
	})

	if opts.ConfigPath != "" {
		_ = addFileRedacted(zw, "config/node.json", opts.ConfigPath)
	}
	if opts.StatusPath != "" {
		_ = addFile(zw, "run/status.json", opts.StatusPath)
	}

	if opts.LogFilePath != "" {
		if tail, err := tailFile(opts.LogFilePath, opts.TailLines); err == nil && len(tail) > 0 {
			_ = addBytes(zw, "logs/tail.log", tail)
		}
	}

	if opts.IncludeAddressBook && opts.AddressBookPath != "" {
		if b, err := os.ReadFile(opts.AddressBookPath); err == nil {
			red, _ := redactAddressBookJSON(b)
			_ = addBytes(zw, "data/addressbook.json", red)
		}
	} else if opts.AddressBookPath != "" {
		if b, err := os.ReadFile(opts.AddressBookPath); err == nil {
			meta := addressBookMeta(b)
			_ = addJSON(zw, "data/addressbook.meta.json", meta)
		}
	}

	if opts.PcapPath != "" {
		if meta, err := pcapMeta(opts.PcapPath); err == nil && meta != nil {
			_ = addJSON(zw, "run/pcap.meta.json", meta)
		}
	}

	return outPath, nil
}

func buildInfo() any {
	if bi, ok := debug.ReadBuildInfo(); ok && bi != nil {
		out := map[string]any{
			"path":    bi.Path,
			"main":    bi.Main.Path,
			"version": bi.Main.Version,
		}
		return out
	}
	return nil
}

func addFile(zw *zip.Writer, name string, srcPath string) error {
	b, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return addBytes(zw, name, b)
}

func addFileRedacted(zw *zip.Writer, name string, srcPath string) error {
	b, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	red, _ := redactConfigJSON(b)
	return addBytes(zw, name, red)
}

func addBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func addJSON(zw *zip.Writer, name string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return addBytes(zw, name, b)
}

func readTailWindow(path string, maxBytes int64) (data []byte, partial bool, fileSize int64, err error) {
	if path == "" || maxBytes <= 0 {
		return nil, false, 0, ErrBadRequest
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, 0, err
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return nil, false, 0, err
	}
	fileSize = stat.Size()
	var start int64
	if fileSize > maxBytes {
		start = fileSize - maxBytes
		partial = true
	}
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return nil, false, fileSize, err
		}
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, partial, fileSize, err
	}
	if start > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 && i+1 < len(b) {
			b = b[i+1:]
		}
	}
	return b, partial, fileSize, nil
}

func tailFile(path string, maxLines int) ([]byte, error) {
	b, _, _, err := readTailWindow(path, MaxTailBytes)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(b, []byte{'\n'})
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	out := bytes.Join(lines, []byte{'\n'})
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}
	return append(out, '\n'), nil
}

func pcapMeta(path string) (any, error) {
	b, partial, size, err := readTailWindow(path, MaxTailBytes)
	if err != nil {
		return nil, err
	}

	type rec struct {
		Dir             string `json:"dir"`
		TSMs            int64  `json:"ts_ms"`
		PeerIDRemote    string `json:"peer_id_remote,omitempty"`
		EnvelopeCBORB64 string `json:"envelope_cbor_b64"`
	}

	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	total := 0
	parseErrors := 0
	inCount := 0
	outCount := 0
	unknownCount := 0
	var minTs int64
	var maxTs int64
	peerSet := map[string]bool{}
	var approxBytes int64

	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		total++
		var r rec
		if err := json.Unmarshal(line, &r); err != nil {
			parseErrors++
			continue
		}
		switch r.Dir {
		case "in":
			inCount++
		case "out":
			outCount++
		default:
			unknownCount++
		}
		if r.TSMs != 0 {
			if minTs == 0 || r.TSMs < minTs {
				minTs = r.TSMs
			}
			if maxTs == 0 || r.TSMs > maxTs {
				maxTs = r.TSMs
			}
		}
		if r.PeerIDRemote != "" {
			peerSet[r.PeerIDRemote] = true
		}
		if r.EnvelopeCBORB64 != "" {
			approxBytes += int64(base64.StdEncoding.DecodedLen(len(r.EnvelopeCBORB64)))
		}
	}

	return map[string]any{
		"path":                        path,
		"file_size_bytes":             size,
		"partial":                     partial,
		"scanned_bytes":               len(b),
		"records_scanned":             total,
		"records_in":                  inCount,
		"records_out":                 outCount,
		"records_unknown":             unknownCount,
		"parse_errors":                parseErrors,
		"ts_ms_min":                   minTs,
		"ts_ms_max":                   maxTs,
		"unique_peers_count":          len(peerSet),
		"envelope_bytes_approx_total": approxBytes,
	}, nil
}

func redactConfigJSON(b []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return b, err
	}
	// config v1: нет явных секретов, но на будущее оставляем место для редактирования
	return json.MarshalIndent(obj, "", "  ")
}

func redactAddressBookJSON(b []byte) ([]byte, error) {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return b, err
	}
	entries, ok := obj["entries"].([]any)
	if !ok {
		return json.MarshalIndent(obj, "", "  ")
	}
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		// убираем произвольные notes из бандла поддержки: часто содержат чувствительные детали
		delete(m, "note")
	}
	obj["entries"] = entries
	return json.MarshalIndent(obj, "", "  ")
}

func addressBookMeta(b []byte) any {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return map[string]any{"error": "ERR_ADDRESSBOOK_DECODE"}
	}
	entries, _ := obj["entries"].([]any)
	revoked, _ := obj["revoked_identity_ids"].([]any)
	depr, _ := obj["deprecated_identity_ids"].([]any)
	return map[string]any{
		"entries_count":                 len(entries),
		"revoked_identity_ids_count":    len(revoked),
		"deprecated_identity_ids_count": len(depr),
	}
}
