// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	stdlog "log"

	"google.golang.org/grpc/grpclog"

	"github.com/rs/zerolog"
)

var (
	FileLogger       zerolog.Logger
	loggerConfigured atomic.Bool
	fileLevelWriter  zerolog.LevelWriter
)

// ParseLogLevel converts a textual log level into a zerolog.Level, defaulting to info.
func ParseLogLevel(level string) zerolog.Level {
	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil {
		return zerolog.InfoLevel
	}
	return lvl
}

// CreateFileLogger configures the global loggers writing to both console and file targets.
func CreateFileLogger(logpath string, level zerolog.Level) zerolog.Logger {
	if level == zerolog.NoLevel {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Ensure the directory exists.
	dir := filepath.Dir(logpath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(fmt.Errorf("failed to create log directory: %w", err))
	}

	logFile, err := os.OpenFile(logpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(fmt.Errorf("failed to open log file: %w", err))
	}

	fileConsoleWriter := zerolog.ConsoleWriter{
		Out:        logFile,
		TimeFormat: "2006-01-02 15:04:05",
		NoColor:    true,
	}

	fileLevelWriter = zerolog.MultiLevelWriter(fileConsoleWriter)
	// multiWriter := zerolog.MultiLevelWriter(consoleWriter, fileConsoleWriter)
	// We only write to file to avoid corrupting the TUI.
	multiWriter := zerolog.MultiLevelWriter(fileConsoleWriter)

	logger := zerolog.New(multiWriter).With().Timestamp().Logger().Level(level)

	// Redirect standard library generic logs to the file logger
	stdlog.SetOutput(logFile)
	stdlog.SetFlags(stdlog.LstdFlags | stdlog.Lmicroseconds | stdlog.Lshortfile)

	// Redirect gRPC logs to the file logger as well.
	// We need a separate logger for gRPC because it requires a specific interface.
	// We use a simple wrapper around our zerolog logger for this purpose,
	// or just direct the standard log to the file (which gRPC uses by default usually,
	// but explicit SetLoggerV2 is safer if it tries to be smart).
	// Ideally, for simplicity and safety against TUI corruption, we just pipe it to the file.
	gLogger := grpclog.NewLoggerV2(logFile, logFile, logFile)
	grpclog.SetLoggerV2(gLogger)

	FileLogger = logger
	loggerConfigured.Store(true)

	return logger
}

// NamedLogger returns a child logger annotated with the given component name.
func NamedLogger(component string) zerolog.Logger {
	if !loggerConfigured.Load() {
		base := zerolog.New(os.Stdout).With().Timestamp().Logger()
		if strings.TrimSpace(component) == "" {
			return base
		}
		return base.With().Str("component", component).Logger()
	}
	component = strings.TrimSpace(component)
	writer := fileLevelWriter
	if writer == nil {
		writer = zerolog.MultiLevelWriter(os.Stdout)
	}
	if component == "" {
		return zerolog.New(writer).With().Timestamp().Logger()
	}
	return zerolog.New(writer).With().Timestamp().Str("component", component).Logger()
}
