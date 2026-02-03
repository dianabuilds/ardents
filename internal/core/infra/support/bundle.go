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
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return "", err
	}

	f, err := os.Create(outPath) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	writeBundleMeta(zw, opts, outPath)
	writeBundleFiles(zw, opts)

	return outPath, nil
}

func writeBundleMeta(zw *zip.Writer, opts BundleOptions, outPath string) {
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
}

func writeBundleFiles(zw *zip.Writer, opts BundleOptions) {
	if opts.ConfigPath != "" {
		_ = addFileRedacted(zw, "config/node.json", opts.ConfigPath)
	}
	if opts.StatusPath != "" {
		_ = addFile(zw, "run/status.json", opts.StatusPath)
	}
	addLogTail(zw, opts.LogFilePath, opts.TailLines)
	addAddressBook(zw, opts.AddressBookPath, opts.IncludeAddressBook)
	addPcapMeta(zw, opts.PcapPath)
}

func addLogTail(zw *zip.Writer, path string, lines int) {
	if path == "" {
		return
	}
	if tail, err := tailFile(path, lines); err == nil && len(tail) > 0 {
		_ = addBytes(zw, "logs/tail.log", tail)
	}
}

func addAddressBook(zw *zip.Writer, path string, include bool) {
	if path == "" {
		return
	}
	if include {
		// #nosec G304 -- path is controlled by app dirs.
		if b, err := os.ReadFile(path); err == nil {
			red, _ := redactAddressBookJSON(b)
			_ = addBytes(zw, "data/addressbook.json", red)
		}
		return
	}
	// #nosec G304 -- path is controlled by app dirs.
	if b, err := os.ReadFile(path); err == nil {
		meta := addressBookMeta(b)
		_ = addJSON(zw, "data/addressbook.meta.json", meta)
	}
}

func addPcapMeta(zw *zip.Writer, path string) {
	if path == "" {
		return
	}
	if meta, err := pcapMeta(path); err == nil && meta != nil {
		_ = addJSON(zw, "run/pcap.meta.json", meta)
	}
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
	b, err := os.ReadFile(srcPath) // #nosec G304 -- path is controlled by app dirs.
	if err != nil {
		return err
	}
	return addBytes(zw, name, b)
}

func addFileRedacted(zw *zip.Writer, name string, srcPath string) error {
	b, err := os.ReadFile(srcPath) // #nosec G304 -- path is controlled by app dirs.
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
	f, err := os.Open(path) // #nosec G304 -- path is controlled by app dirs.
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
	stats := scanPcapLines(b)
	return map[string]any{
		"path":                        path,
		"file_size_bytes":             size,
		"partial":                     partial,
		"scanned_bytes":               len(b),
		"records_scanned":             stats.total,
		"records_in":                  stats.inCount,
		"records_out":                 stats.outCount,
		"records_unknown":             stats.unknownCount,
		"parse_errors":                stats.parseErrors,
		"ts_ms_min":                   stats.minTs,
		"ts_ms_max":                   stats.maxTs,
		"unique_peers_count":          len(stats.peerSet),
		"envelope_bytes_approx_total": stats.approxBytes,
	}, nil
}

type pcapScanStats struct {
	total        int
	parseErrors  int
	inCount      int
	outCount     int
	unknownCount int
	minTs        int64
	maxTs        int64
	approxBytes  int64
	peerSet      map[string]bool
}

func scanPcapLines(b []byte) pcapScanStats {
	type rec struct {
		Dir             string `json:"dir"`
		TSMs            int64  `json:"ts_ms"`
		PeerIDRemote    string `json:"peer_id_remote,omitempty"`
		EnvelopeCBORB64 string `json:"envelope_cbor_b64"`
	}

	stats := pcapScanStats{
		peerSet: map[string]bool{},
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		stats.total++
		var r rec
		if err := json.Unmarshal(line, &r); err != nil {
			stats.parseErrors++
			continue
		}
		switch r.Dir {
		case "in":
			stats.inCount++
		case "out":
			stats.outCount++
		default:
			stats.unknownCount++
		}
		if r.TSMs != 0 {
			if stats.minTs == 0 || r.TSMs < stats.minTs {
				stats.minTs = r.TSMs
			}
			if stats.maxTs == 0 || r.TSMs > stats.maxTs {
				stats.maxTs = r.TSMs
			}
		}
		if r.PeerIDRemote != "" {
			stats.peerSet[r.PeerIDRemote] = true
		}
		if r.EnvelopeCBORB64 != "" {
			stats.approxBytes += int64(base64.StdEncoding.DecodedLen(len(r.EnvelopeCBORB64)))
		}
	}
	return stats
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
