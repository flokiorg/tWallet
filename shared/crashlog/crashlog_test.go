package crashlog_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flokiorg/twallet/shared/crashlog"
)

// TestStderrRedirectCapturesPanic uses a subprocess to trigger a real panic
// and verifies that the panic output is captured in the crash log file.
func TestStderrRedirectCapturesPanic(t *testing.T) {
	// When the test binary is re-invoked with this env var, it will panic.
	if os.Getenv("CRASHLOG_TEST_CRASH") == "1" {
		crashLog := os.Getenv("CRASHLOG_TEST_CRASH_LOG")
		f, err := os.OpenFile(crashLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			os.Exit(2)
		}
		if err := crashlog.RedirectStderr(f); err != nil {
			os.Exit(3)
		}
		// Trigger a real panic — the runtime writes the stack trace to stderr
		// which is now pointed at the crash log file.
		panic("test crash for crashlog verification")
	}

	// --- Parent process: spawn a child that will panic ---
	tmpDir := t.TempDir()
	crashLogPath := filepath.Join(tmpDir, "crash.log")

	cmd := exec.Command(os.Args[0], "-test.run=^TestStderrRedirectCapturesPanic$")
	cmd.Env = append(os.Environ(),
		"CRASHLOG_TEST_CRASH=1",
		"CRASHLOG_TEST_CRASH_LOG="+crashLogPath,
	)
	// We expect the subprocess to exit with a non-zero code (panic).
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit with error (panic), but it succeeded")
	}

	// Verify the crash log exists and contains the panic message.
	data, err := os.ReadFile(crashLogPath)
	if err != nil {
		t.Fatalf("crash.log was not created: %v", err)
	}

	content := string(data)
	t.Logf("crash.log contents (%d bytes):\n%s", len(data), content)

	if !strings.Contains(content, "test crash for crashlog verification") {
		t.Error("crash.log does not contain the panic message")
	}
	if !strings.Contains(content, "goroutine") {
		t.Error("crash.log does not contain a goroutine stack trace")
	}
}
