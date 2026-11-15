package wallet

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	logPollInterval    = 750 * time.Millisecond
	maxInitialLogBytes = int64(2 * 1024 * 1024)
)

func (w *Wallet) startLogTail() {
	if w.load == nil || w.load.AppConfig == nil || w.logView == nil || w.logQuit == nil {
		return
	}

	networkName := "unknown"
	if w.load.AppConfig.Network != nil {
		networkName = w.load.AppConfig.Network.Name
	}
	w.logPath = filepath.Join(w.load.AppConfig.Walletdir, "logs", "flokicoin", networkName, "flnd.log")
	w.setLogStatus(fmt.Sprintf("Loading log from %s", w.logPath))

	go w.tailLog()
}

func (w *Wallet) tailLog() {
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()

	var offset int64

	for {
		select {
		case <-w.logQuit:
			return
		case <-ticker.C:
		}

		info, err := os.Stat(w.logPath)
		if err != nil {
			if os.IsNotExist(err) {
				w.setLogStatus(fmt.Sprintf("Waiting for log file at %s", w.logPath))
			} else {
				w.setLogStatus(fmt.Sprintf("Log unavailable: %v", err))
			}
			continue
		}

		size := info.Size()
		if size < offset {
			offset = 0
		}

		f, err := os.Open(w.logPath)
		if err != nil {
			w.setLogStatus(fmt.Sprintf("Unable to open log file: %v", err))
			continue
		}

		if offset == 0 {
			start := int64(0)
			if size > maxInitialLogBytes {
				start = size - maxInitialLogBytes
			}
			if start > 0 {
				if _, err := f.Seek(start, io.SeekStart); err != nil {
					f.Close()
					offset = 0
					continue
				}
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				w.setLogStatus(fmt.Sprintf("Unable to read log file: %v", err))
				continue
			}
			lines := w.readLogLines(data)
			if start > 0 && len(lines) > 0 {
				lines = lines[1:]
			}
			if len(lines) > 0 {
				w.replaceLogLines(lines)
				w.logReady = true
			} else if !w.logReady {
				w.setLogStatus("Log file is empty.")
			}
			offset = size
			continue
		}

		if size == offset {
			f.Close()
			continue
		}

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			offset = 0
			continue
		}

		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			w.setLogStatus(fmt.Sprintf("Unable to read log file: %v", err))
			continue
		}

		offset += int64(len(data))
		lines := w.readLogLines(data)
		if len(lines) > 0 {
			w.appendLogLines(lines)
			w.logReady = true
		}
	}
}

func (w *Wallet) replaceLogLines(lines []string) {
	if len(lines) == 0 {
		return
	}
	if w.logMaxLine > 0 && len(lines) > w.logMaxLine {
		lines = lines[len(lines)-w.logMaxLine:]
	}

	w.logMu.Lock()
	w.logLines = append([]string{}, lines...)
	w.logStatus = ""
	text := strings.Join(w.logLines, "\n")
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) appendLogLines(lines []string) {
	if len(lines) == 0 {
		return
	}

	w.logMu.Lock()
	w.logStatus = ""
	if w.logMaxLine > 0 {
		total := len(w.logLines) + len(lines)
		if total > w.logMaxLine {
			drop := total - w.logMaxLine
			if drop >= len(w.logLines) {
				w.logLines = append([]string{}, lines...)
			} else {
				w.logLines = append(append([]string{}, w.logLines[drop:]...), lines...)
			}
		} else {
			w.logLines = append(w.logLines, lines...)
		}
	} else {
		w.logLines = append(w.logLines, lines...)
	}
	text := strings.Join(w.logLines, "\n")
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) setLogStatus(message string) {
	w.logMu.Lock()
	if w.logStatus == message {
		w.logMu.Unlock()
		return
	}
	w.logStatus = message
	w.logReady = false
	w.logLines = []string{message}
	text := message
	w.logMu.Unlock()

	w.updateLogView(text)
}

func (w *Wallet) updateLogView(text string) {
	if w.load == nil || w.load.Application == nil {
		return
	}
	w.load.Application.QueueUpdateDraw(func() {
		if w.logView != nil {
			w.logView.SetText(text)
		}
	})
}

func (w *Wallet) readLogLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil && w.load != nil {
		w.load.Logger.Warn().Err(err).Msg("log scanner error")
	}
	return lines
}
