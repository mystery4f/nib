// Package niblog writes the session log. A windowed build has no console, so
// this file is the only evidence left when startup fails.
package niblog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger appends timestamped lines to a file. A nil or failed logger discards.
type Logger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// Open creates the log under the user's cache directory and appends to it. When
// that fails the returned logger still works and discards everything, and err
// keeps the reason.
func Open() (*Logger, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return &Logger{}, err
	}
	dir = filepath.Join(dir, "nib")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Logger{}, err
	}
	path := filepath.Join(dir, "nib.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return &Logger{}, err
	}
	return &Logger{file: file, path: path}, nil
}

// Path is where the log is written, empty when nothing is written.
func (l *Logger) Path() string { return l.path }

// Infof records a line of narrative.
func (l *Logger) Infof(format string, args ...any) { l.write("info", format, args...) }

// Errorf records a failure.
func (l *Logger) Errorf(format string, args ...any) { l.write("error", format, args...) }

// Close flushes and releases the file.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Logger) write(level, format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.file, "%s %-5s %s\n", time.Now().Format(time.RFC3339), level, line)
	l.file.Sync()
}
