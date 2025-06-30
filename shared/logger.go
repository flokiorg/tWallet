// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package shared

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

var (
	ConsoleLogger zerolog.Logger
	FileLogger    zerolog.Logger
)

func CreateFileLogger(logpath string) zerolog.Logger {
	consoleWriter := zerolog.ConsoleWriter{
		Out: os.Stdout,
	}

	ConsoleLogger = zerolog.New(consoleWriter).With().Timestamp().Logger()

	// Ensure the directory exists
	dir := filepath.Dir(logpath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(fmt.Errorf("failed to create log directory: %w", err))
	}

	logFile, err := os.OpenFile(logpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Errorf("failed to open log file: %w", err))
	}

	multiWriter := zerolog.MultiLevelWriter(
		zerolog.ConsoleWriter{Out: logFile, TimeFormat: "2006-01-02 15:04:05", NoColor: true},
	)

	return zerolog.New(multiWriter).With().Timestamp().Logger().Level(zerolog.TraceLevel)
}
