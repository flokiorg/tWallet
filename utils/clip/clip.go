package clip

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/term"

	"github.com/atotto/clipboard"
)

type Method string

const (
	MethodOSC52       Method = "osc52"
	MethodAtotto      Method = "go-clipboard"
	MethodOSCommand   Method = "os-command"
	MethodUnsupported Method = "unsupported"
)

// CopyText tries several methods to copy text to the user's clipboard.
// Returns the method used (useful for logging/UX) or an error.
func CopyText(text string) (Method, error) {
	if text == "" {
		return MethodUnsupported, errors.New("empty text")
	}

	// Helper wrappers to match signatures
	tryOSC52 := func() error {
		if canTryOSC52(os.Stdout, text) {
			return writeOSC52(os.Stdout, text)
		}
		return errors.New("cannot try OSC 52")
	}

	tryAtotto := func() error {
		return clipboard.WriteAll(text)
	}

	tryOSCommand := func() error {
		return tryOSCommandClipboard(text)
	}

	// Strategy pattern based on OS
	var strategies []struct {
		method Method
		fn     func() error
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		// On macOS/Windows, native clipboard is reliable and preferred locally.
		strategies = []struct {
			method Method
			fn     func() error
		}{
			{MethodAtotto, tryAtotto},
			{MethodOSCommand, tryOSCommand},
			{MethodOSC52, tryOSC52},
		}
	} else {
		// On Linux/Unix, prioritizing OSC 52 is safer for SSH/Headless scenarios
		// to avoid breaking existing workflows dependent on it.
		strategies = []struct {
			method Method
			fn     func() error
		}{
			{MethodOSC52, tryOSC52},
			{MethodAtotto, tryAtotto},
			{MethodOSCommand, tryOSCommand},
		}
	}

	for _, s := range strategies {
		if err := s.fn(); err == nil {
			return s.method, nil
		}
	}

	return MethodUnsupported, fmt.Errorf("could not copy using any method (%v)", strategies)
}

func canTryOSC52(w io.Writer, text string) bool {
	// Only attempt if stdout is a terminal.
	if f, ok := w.(*os.File); ok {
		if !term.IsTerminal(int(f.Fd())) {
			return false
		}
	}

	// Heuristic: avoid emitting OSC 52 in "dumb" terminals.
	termEnv := os.Getenv("TERM")
	if termEnv == "" || termEnv == "dumb" {
		return false
	}

	// Practical payload limit: terminals often cap OSC 52 size.
	// 8KB is conservative; increase if you want.
	const maxBytes = 8 * 1024
	if len(text) > maxBytes {
		return false
	}

	return true
}

func writeOSC52(w io.Writer, text string) error {
	b64 := base64.StdEncoding.EncodeToString([]byte(text))

	// Standard OSC 52 sequence: ESC ] 52 ; c ; <base64> BEL
	seq := "\x1b]52;c;" + b64 + "\x07"

	// If inside tmux/screen, wrap for passthrough.
	if os.Getenv("TMUX") != "" {
		// tmux passthrough: ESC P tmux; ESC ESC ] ... BEL ESC \
		seq = "\x1bPtmux;\x1b" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	} else if os.Getenv("STY") != "" {
		// GNU screen passthrough: ESC P ... ESC \
		seq = "\x1bP" + seq + "\x1b\\"
	}

	_, err := io.WriteString(w, seq)
	return err
}

func tryOSCommandClipboard(text string) error {
	switch runtime.GOOS {
	case "darwin":
		// pbcopy is present on macOS by default.
		return runClipboardCommand("pbcopy", nil, text)
	case "windows":
		// clip.exe is present on Windows by default.
		// Use cmd to find it reliably in PATH/System32.
		return runClipboardCommand("cmd", []string{"/c", "clip"}, text)
	default:
		// Linux/BSD: no guaranteed built-in clipboard command without extra packages,
		// so we intentionally don't try xclip/xsel/wl-copy unless you decide it's acceptable.
		return errors.New("no built-in clipboard command for this OS")
	}
}

func runClipboardCommand(name string, args []string, stdinText string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewBufferString(stdinText)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
