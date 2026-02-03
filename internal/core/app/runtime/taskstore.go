package runtime

import (
	"crypto/sha256"
	"sync"
	"time"
)

type taskEntry struct {
	hash    [32]byte
	resps   [][]byte
	created time.Time
}

type TaskStore struct {
	mu        sync.Mutex
	byClient  map[string]taskEntry
	seenTasks map[string]time.Time
	byTask    map[string]taskEntry
	ttl       time.Duration
	nowFn     func() time.Time
}

func NewTaskStore(ttl time.Duration) *TaskStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &TaskStore{
		byClient:  make(map[string]taskEntry),
		seenTasks: make(map[string]time.Time),
		byTask:    make(map[string]taskEntry),
		ttl:       ttl,
		nowFn:     time.Now,
	}
}

func (s *TaskStore) Check(taskID string, clientRequestID string, payload []byte) (dupResps [][]byte, errCode string) {
	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	if taskID != "" {
		if _, ok := s.seenTasks[taskID]; ok {
			return nil, "ERR_TASK_REJECTED"
		}
	}
	if clientRequestID == "" {
		return nil, ""
	}
	if entry, ok := s.byClient[clientRequestID]; ok {
		hash := sha256.Sum256(payload)
		if entry.hash == hash {
			return entry.resps, ""
		}
		return nil, "ERR_TASK_REJECTED"
	}
	return nil, ""
}

func (s *TaskStore) Store(taskID string, clientRequestID string, payload []byte, resps [][]byte) {
	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	if taskID != "" {
		s.seenTasks[taskID] = now
	}
	if clientRequestID == "" {
		return
	}
	hash := sha256.Sum256(payload)
	s.byClient[clientRequestID] = taskEntry{
		hash:    hash,
		resps:   cloneResps(resps),
		created: now,
	}
}

func (s *TaskStore) StoreResponse(taskID string, resp []byte) {
	if taskID == "" || resp == nil {
		return
	}
	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	entry := s.byTask[taskID]
	entry.resps = append(entry.resps, cloneResps([][]byte{resp})...)
	if len(entry.resps) > 16 {
		entry.resps = entry.resps[len(entry.resps)-16:]
	}
	entry.created = now
	s.byTask[taskID] = entry
}

func (s *TaskStore) Responses(taskID string) [][]byte {
	if taskID == "" {
		return nil
	}
	now := s.nowFn()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	entry, ok := s.byTask[taskID]
	if !ok {
		return nil
	}
	return cloneResps(entry.resps)
}

func (s *TaskStore) gcLocked(now time.Time) {
	exp := now.Add(-s.ttl)
	for k, v := range s.byClient {
		if v.created.Before(exp) {
			delete(s.byClient, k)
		}
	}
	for k, t := range s.seenTasks {
		if t.Before(exp) {
			delete(s.seenTasks, k)
		}
	}
	for k, v := range s.byTask {
		if v.created.Before(exp) {
			delete(s.byTask, k)
		}
	}
}

func cloneResps(in [][]byte) [][]byte {
	out := make([][]byte, 0, len(in))
	for _, b := range in {
		if b == nil {
			out = append(out, nil)
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		out = append(out, cp)
	}
	return out
}
