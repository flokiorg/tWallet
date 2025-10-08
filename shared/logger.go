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

	"github.com/rs/zerolog"
)

var (
	ConsoleLogger    zerolog.Logger
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

	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
	}

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
	multiWriter := zerolog.MultiLevelWriter(consoleWriter, fileConsoleWriter)

	logger := zerolog.New(multiWriter).With().Timestamp().Logger().Level(level)

	ConsoleLogger = zerolog.New(consoleWriter).With().Timestamp().Logger().Level(level)
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
