package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dianabuilds/ardents/internal/shared/perm"
)

type Logger struct {
	mu       *sync.Mutex
	format   string
	file     *os.File
	filePath string
}

func New() *Logger {
	return NewWithOptions("json", "")
}

func NewWithOptions(format string, filePath string) *Logger {
	if format == "" {
		format = "json"
	}
	return &Logger{
		mu:       &sync.Mutex{},
		format:   format,
		filePath: filePath,
	}
}

func (l *Logger) Event(level, component, event, peerID, msgID, errorCode string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	line, ok := l.renderLine(level, component, event, peerID, msgID, errorCode)
	if !ok {
		return
	}
	_, _ = os.Stdout.Write(append(line, '\n'))
	if l.filePath == "" {
		return
	}
	if l.file == nil {
		if err := os.MkdirAll(filepath.Dir(l.filePath), 0o755); err != nil {
			return
		}
		f, err := perm.OpenOwnerOnly(l.filePath)
		if err != nil {
			return
		}
		l.file = f
	}
	_, _ = l.file.Write(append(line, '\n'))
}

func EnforceRetention(path string, maxAge time.Duration, maxSizeBytes int64) {
	if path == "" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if maxAge > 0 {
		if time.Since(info.ModTime()) > maxAge {
			_ = os.Remove(path)
			return
		}
	}
	if maxSizeBytes > 0 && info.Size() > maxSizeBytes {
		_ = os.Remove(path)
	}
}

func (l *Logger) renderLine(level, component, event, peerID, msgID, errorCode string) ([]byte, bool) {
	nowMs := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	if l.format == "text" {
		// v1: текстовый формат допустим только как локальный режим; wire-инварианты не затрагивает.
		b := []byte(event)
		if component != "" {
			b = []byte(component + " " + event)
		}
		return []byte(
			time.UnixMilli(nowMs).UTC().Format(time.RFC3339Nano) + " " + level + " " + string(b),
		), true
	}
	obj := map[string]any{
		"ts_ms":     nowMs,
		"level":     level,
		"component": component,
		"event":     event,
	}
	if peerID != "" {
		obj["peer_id"] = peerID
	}
	if msgID != "" {
		obj["msg_id"] = msgID
	}
	if errorCode != "" {
		obj["error_code"] = errorCode
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return data, true
}
