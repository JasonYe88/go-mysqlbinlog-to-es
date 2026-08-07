package river

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const defaultLogRingSize = 2000

// LogRing keeps recent log lines in memory for the Admin UI.
type LogRing struct {
	mu   sync.RWMutex
	buf  []string
	size int
	pos  int
	full bool
}

// GlobalLogRing is process-wide; filled by the multi-writer log handler.
var GlobalLogRing = NewLogRing(defaultLogRingSize)

func NewLogRing(size int) *LogRing {
	if size <= 0 {
		size = defaultLogRingSize
	}
	return &LogRing{
		buf:  make([]string, size),
		size: size,
	}
}

// Write implements io.Writer so it can wrap go-log StreamHandler output.
func (r *LogRing) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	parts := strings.Split(string(p), "\n")
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, line := range parts {
		if line == "" && i == len(parts)-1 {
			continue
		}
		r.buf[r.pos] = line
		r.pos = (r.pos + 1) % r.size
		if r.pos == 0 {
			r.full = true
		}
	}
	return len(p), nil
}

// Lines returns the newest lines, optionally filtered by substring (case-insensitive).
func (r *LogRing) Lines(limit int, filter string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []string
	if !r.full {
		all = append(all, r.buf[:r.pos]...)
	} else {
		all = append(all, r.buf[r.pos:]...)
		all = append(all, r.buf[:r.pos]...)
	}

	filter = strings.TrimSpace(filter)
	if filter != "" {
		f := strings.ToLower(filter)
		filtered := make([]string, 0, len(all))
		for _, line := range all {
			if strings.Contains(strings.ToLower(line), f) {
				filtered = append(filtered, line)
			}
		}
		all = filtered
	}

	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	if limit == 0 {
		return []string{}
	}
	return all[len(all)-limit:]
}

func handleMemoryLogs(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	limit, _ := strconv.Atoi(q.Get("tail"))
	if limit <= 0 {
		limit = 300
	}
	if limit > 2000 {
		limit = 2000
	}
	lines := GlobalLogRing.Lines(limit, q.Get("filter"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"source": "buffer",
		"lines":  lines,
	})
}
