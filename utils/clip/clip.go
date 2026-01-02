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

	// 1) Try OSC 52 (terminal-mediated clipboard; no installs).
	if canTryOSC52(os.Stdout, text) {
		if err := writeOSC52(os.Stdout, text); err == nil {
			return MethodOSC52, nil
		}
		// If it fails, continue to next methods.
	}

	// 2) Try Go clipboard library (works well on Windows/macOS; Linux depends on environment).
	if err := clipboard.WriteAll(text); err == nil {
		return MethodAtotto, nil
	}

	// 3) Try built-in OS commands (no extra installs in typical environments).
	if err := tryOSCommandClipboard(text); err == nil {
		return MethodOSCommand, nil
	}

	return MethodUnsupported, fmt.Errorf("could not copy to clipboard via osc52/go-lib/os-command; fallback: show text for manual copy")
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
