package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	LogDir     string
	Filename   string // e.g. "netchecker.log"
	MaxSizeMB  int    // 10
	MaxBackups int    // 10
	Compress   bool   // true
	AlsoStdout bool   // true in dev
}

func Init(opts Options) (string, error) {
	if opts.LogDir == "" {
		opts.LogDir = "."
	}
	if opts.Filename == "" {
		opts.Filename = "netchecker.log"
	}
	if opts.MaxSizeMB <= 0 {
		opts.MaxSizeMB = 10
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = 10
	}

	if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(opts.LogDir, opts.Filename)

	rot := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		Compress:   opts.Compress,
	}

	var w io.Writer = rot
	if opts.AlsoStdout {
		w = io.MultiWriter(os.Stdout, rot)
	}

	log.SetOutput(w)
	log.SetFlags(0) // мы рисуем время сами

	Info("logging", "init logfile=%s maxSizeMB=%d maxBackups=%d compress=%t stdout=%t",
		path, opts.MaxSizeMB, opts.MaxBackups, opts.Compress, opts.AlsoStdout)

	return path, nil
}

func ts() string {
	return time.Now().Format("2006-01-02T15:04:05.000-07:00")
}

func Info(component, msg string, args ...any) {
	log.Printf("%s INFO  %-8s %s", ts(), component, fmt.Sprintf(msg, args...))
}

func Warn(component, msg string, args ...any) {
	log.Printf("%s WARN  %-8s %s", ts(), component, fmt.Sprintf(msg, args...))
}

func Error(component, msg string, args ...any) {
	log.Printf("%s ERROR %-8s %s", ts(), component, fmt.Sprintf(msg, args...))
}

func Debug(component, msg string, args ...any) {
	log.Printf("%s DEBUG %-8s %s", ts(), component, fmt.Sprintf(msg, args...))
}
