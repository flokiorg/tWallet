// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package utils

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/flokiorg/flnd/lnrpc"
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

func GetAddressTypesFromName(name string) (used lnrpc.AddressType, unused lnrpc.AddressType, err error) {
	switch name {
	case "segwit":
		return lnrpc.AddressType_WITNESS_PUBKEY_HASH, lnrpc.AddressType_UNUSED_WITNESS_PUBKEY_HASH, nil
	case "nested-segwit":
		return lnrpc.AddressType_NESTED_PUBKEY_HASH, lnrpc.AddressType_UNUSED_NESTED_PUBKEY_HASH, nil
	case "taproot":
		return lnrpc.AddressType_TAPROOT_PUBKEY, lnrpc.AddressType_UNUSED_TAPROOT_PUBKEY, nil
	default:
		return 0, 0, fmt.Errorf("unknown address type: %s", name)
	}
}

func IsTaprootAddressType(t lnrpc.AddressType) bool {
	switch t {
	case lnrpc.AddressType_TAPROOT_PUBKEY, lnrpc.AddressType_UNUSED_TAPROOT_PUBKEY:
		return true
	default:
		return false
	}
}

func FormatBootError(err error) string {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			if errors.Is(sysErr.Err, syscall.EADDRINUSE) {
				return fmt.Sprintf("Another instance is already running: %v", err.Error())
			}
		}
	}
	return err.Error()
}
