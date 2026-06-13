// Package log 提供后端日志持久化能力。
//
// RotatingWriter 实现 io.Writer，将日志写入 logs/ 目录下的带时间戳文件，
// 单文件超过 10MB 时自动切换到新文件，保证日志可追溯且不耗尽磁盘。
package log

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultMaxSize 单文件默认上限（10MB）。
	DefaultMaxSize = 10 * 1024 * 1024
)

// RotatingWriter 是一个线程安全的旋转文件写入器。
//
// 写入前检查当前文件大小，超过 maxSize 则关闭旧文件、创建新文件。
// 新文件名格式：log-YYYY-MM-DD-HH-MM-SS.log。
type RotatingWriter struct {
	dir      string
	maxSize  int64
	mu       sync.Mutex
	file     *os.File
	currSize int64
}

// NewRotatingWriter 创建旋转文件写入器。
//
// dir 为日志目录路径（如项目根目录下的 "logs"），不存在则自动创建。
// maxSize 为单文件字节上限，传 0 使用默认值 10MB。
func NewRotatingWriter(dir string, maxSize int64) (*RotatingWriter, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}

	// 确保目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s 失败: %w", dir, err)
	}

	w := &RotatingWriter{
		dir:     dir,
		maxSize: maxSize,
	}

	// 打开首个日志文件
	if err := w.rotate(); err != nil {
		return nil, err
	}

	return w, nil
}

// Write 实现 io.Writer 接口。
//
// 写入前检查是否需要切换文件（当前大小 + 本次写入 > 上限）。
// 单次写入超过 maxSize 时直接切换文件写入，不截断内容。
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 需要切换文件时先关闭旧文件再打开新文件
	if w.currSize+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("切换日志文件失败: %w", err)
		}
	}

	n, err = w.file.Write(p)
	w.currSize += int64(n)
	return n, err
}

// rotate 关闭当前文件（如有），创建新的日志文件。
//
// 调用方需持有 w.mu。
func (w *RotatingWriter) rotate() error {
	// 关闭旧文件
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("关闭日志文件失败: %w", err)
		}
	}

	// 创建新文件：log-YYYY-MM-DD-HH-MM-SS.log
	name := fmt.Sprintf("log-%s.log", time.Now().Format("2006-01-02-15-04-05"))
	f, err := os.OpenFile(filepath.Join(w.dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建日志文件 %s 失败: %w", name, err)
	}

	// 记录当前大小（追加模式下文件可能已存在）
	stat, _ := f.Stat()
	w.file = f
	w.currSize = stat.Size()
	return nil
}

// Close 关闭当前日志文件，释放资源。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
