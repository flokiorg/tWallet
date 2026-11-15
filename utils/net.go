// Copyright (c) 2024 The Flokicoin developers
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package utils

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// ValidateAndNormalizeURI validates and normalizes a URI (IP/domain + optional port).
func ValidateAndNormalizeURI(raw string, defaultPort int) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty string is not valid")
	}

	// 1) Check if this is bracketed IPv6: [IPv6]:port or [IPv6]
	if strings.HasPrefix(raw, "[") {
		return parseBracketedIPv6(raw, defaultPort)
	}

	// 2) Split on ":" to see if there's a port
	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 1:
		// No colon => either domain, IPv4, or raw IPv6 with no port
		host := parts[0]
		if !isHostOrIP(host) {
			return "", fmt.Errorf("invalid hostname or IP: %s", host)
		}
		// Default to port 80
		return fmt.Sprintf("%s:%d", host, defaultPort), nil

	case 2:
		// single colon => host:port (could be domain or IPv4)
		host := parts[0]
		port := parts[1]
		if !isHostOrIP(host) {
			return "", fmt.Errorf("invalid hostname or IP: %s", host)
		}
		if !isValidPort(port) {
			return "", fmt.Errorf("invalid port: %s", port)
		}
		// e.g. "example.com:8080" -> "example.com:8080"
		return fmt.Sprintf("%s:%s", host, port), nil

	default:
		// More than one colon => possible raw IPv6 WITHOUT brackets?
		// In URI form, IPv6 with a port must use brackets, so let's see if it’s a valid raw IPv6 *with no port*:
		// E.g. "2001:db8::1" -> valid IP; "2001:db8::1:9999" -> invalid w/o brackets
		if net.ParseIP(raw) != nil {
			// It's a valid raw IPv6 (no port), default to 80
			return fmt.Sprintf("%s:%d", raw, defaultPort), nil
		}
		return "", fmt.Errorf("invalid format or missing brackets for IPv6 with port: %s", raw)
	}
}

// parseBracketedIPv6 extracts IPv6 + optional port from strings like "[::1]:8080" or "[2001:db8::1]"
func parseBracketedIPv6(raw string, defaultPort int) (string, error) {
	// Must have a closing bracket
	endBracket := strings.Index(raw, "]")
	if endBracket == -1 {
		return "", fmt.Errorf("missing closing bracket in IPv6: %s", raw)
	}

	// Extract the IPv6 portion inside [ ]
	ipv6Part := raw[1:endBracket]
	if net.ParseIP(ipv6Part) == nil {
		return "", fmt.Errorf("invalid IPv6 address: %s", ipv6Part)
	}

	// After the closing bracket, see if we have a port
	remainder := raw[endBracket+1:] // e.g. ":8080" or ""

	if remainder == "" {
		// e.g. "[2001:db8::1]"
		return fmt.Sprintf("%s:%d", ipv6Part, defaultPort), nil
	}

	// Must start with ":"
	if !strings.HasPrefix(remainder, ":") {
		return "", fmt.Errorf("invalid bracketed IPv6 format (missing colon after ]): %s", raw)
	}

	port := remainder[1:]
	if port == "" {
		// e.g. "[2001:db8::1]:"
		return "", fmt.Errorf("empty port after bracketed IPv6: %s", raw)
	}

	if !isValidPort(port) {
		return "", fmt.Errorf("invalid port: %s", port)
	}

	// e.g. "[2001:db8::1]:9000" -> "2001:db8::1:9000"
	return fmt.Sprintf("%s:%s", ipv6Part, port), nil
}

// isHostOrIP returns true if s is a valid domain name OR valid IP (v4 or v6).
func isHostOrIP(s string) bool {
	// First check if it's an IP
	if net.ParseIP(s) != nil {
		return true
	}
	// Otherwise check if it's a valid hostname
	return isValidHostname(s)
}

// isValidHostname checks if s is a valid domain name (RFC 1035-like).
func isValidHostname(h string) bool {
	// Regex to allow [a-z0-9-] segments, up to 253 chars total
	// Simplified check; real-world DNS rules can be more complex.
	var reHostname = regexp.MustCompile(`^(?i:[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*)$`)
	if len(h) > 253 {
		return false
	}
	return reHostname.MatchString(h)
}

// isValidPort checks if p is a valid port number in [1, 65535].
func isValidPort(p string) bool {
	num, err := strconv.Atoi(p)
	if err != nil || num < 1 || num > 65535 {
		return false
	}
	return true
}
