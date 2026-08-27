package nlog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Logger writes to stdout and optionally a debug file.
type Logger struct {
	mu     sync.Mutex
	debug  bool
	file   *os.File
	logger *log.Logger
}

func New(debug bool, path string) (*Logger, error) {
	l := &Logger{debug: debug, logger: log.New(os.Stdout, "", log.LstdFlags)}
	if debug && path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return l, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return l, err
		}
		l.file = f
		l.logger = log.New(io.MultiWriter(os.Stdout, f), "", log.LstdFlags)
	}
	return l, nil
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
	}
}

func (l *Logger) Infof(format string, args ...any) {
	l.logger.Printf(format, args...)
}

func (l *Logger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	l.logger.Printf("[debug] "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.logger.Printf("[error] "+format, args...)
}

func (l *Logger) Printf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}
