package observability

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Logger struct {
	mu *sync.Mutex
}

func New() *Logger {
	return &Logger{mu: &sync.Mutex{}}
}

func (l *Logger) Event(level, component, event, peerID, msgID, errorCode string) {
	if l == nil {
		return
	}
	obj := map[string]any{
		"ts_ms":     time.Now().UTC().UnixNano() / int64(time.Millisecond),
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
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		return
	}
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
