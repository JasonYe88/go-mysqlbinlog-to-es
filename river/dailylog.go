package river

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DailyLogWriter 按天分割的日志写入器，自动清理旧日志
type DailyLogWriter struct {
	mu         sync.Mutex
	dir        string
	prefix     string
	file       *os.File
	currentDay string
	maxDays    int // 保留的最大天数，0表示不清理
}

// NewDailyLogWriter 创建按天分割的日志写入器
// dir: 日志目录
// prefix: 日志文件前缀，例如 "sync"
// maxDays: 保留的最大天数，0表示不清理
func NewDailyLogWriter(dir, prefix string, maxDays int) (*DailyLogWriter, error) {
	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir failed: %v", err)
	}

	w := &DailyLogWriter{
		dir:     dir,
		prefix:  prefix,
		maxDays: maxDays,
	}

	// 打开今天的日志文件
	if err := w.rotate(); err != nil {
		return nil, err
	}

	// 启动清理协程
	if maxDays > 0 {
		go w.cleanupLoop()
	}

	return w, nil
}

// Write 实现 io.Writer 接口
func (w *DailyLogWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// 检查是否需要切换文件
	day := w.getDayString()
	if day != w.currentDay {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	return w.file.Write(p)
}

// getDayString 获取当前日期字符串
func (w *DailyLogWriter) getDayString() string {
	return time.Now().Format("20060102")
}

// getFileName 获取指定日期的日志文件名
func (w *DailyLogWriter) getFileName(day string) string {
	return filepath.Join(w.dir, fmt.Sprintf("%s_%s.log", w.prefix, day))
}

// rotate 切换到新的日志文件
func (w *DailyLogWriter) rotate() error {
	day := w.getDayString()
	filename := w.getFileName(day)

	// 如果已经是打开的文件且日期相同，不需要切换
	if w.file != nil && day == w.currentDay {
		return nil
	}

	// 关闭旧文件
	if w.file != nil {
		w.file.Close()
	}

	// 打开新文件，追加模式
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file failed: %v", err)
	}

	w.file = f
	w.currentDay = day

	return nil
}

// cleanupLoop 定期清理旧日志
func (w *DailyLogWriter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		w.Cleanup()
	}
}

// Cleanup 清理超过 maxDays 的旧日志
func (w *DailyLogWriter) Cleanup() {
	if w.maxDays <= 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -w.maxDays).Format("20060102")

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	prefix := w.prefix + "_"
	suffix := ".log"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}

		// 提取日期部分
		day := strings.TrimPrefix(name, prefix)
		day = strings.TrimSuffix(day, suffix)

		// 如果日期早于截止日期，删除文件
		if day < cutoff {
			path := filepath.Join(w.dir, name)
			os.Remove(path)
		}
	}
}

// Close 关闭日志写入器
func (w *DailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// ListLogs 列出所有日志文件（按日期排序）
func (w *DailyLogWriter) ListLogs() ([]string, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, err
	}

	var logs []string
	prefix := w.prefix + "_"
	suffix := ".log"

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			logs = append(logs, filepath.Join(w.dir, name))
		}
	}

	// 按文件名排序（日期排序）
	sort.Strings(logs)
	return logs, nil
}
