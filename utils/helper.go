// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package utils

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

func GetEnvOrFail(key string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		log.Fatal().Msgf("Error: environment variable %s is not set", key)
	}
	return value
}

func GetEnv[T any](key string, defaultValue T, parseFunc func(string) (T, error)) T {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	parsedValue, err := parseFunc(value)
	if err != nil {
		log.Warn().Msgf("Failed to parse %s, using default value. Error: %v\n", key, err)
		return defaultValue
	}
	return parsedValue
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

func GetFullPath(filename string) (string, error) {
	dir, err := os.Getwd() // Get current working directory
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}
