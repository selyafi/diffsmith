package clipboard

import (
	"bytes"
	"os/exec"
	"runtime"
	"testing"
)

// TestCopyPipesTextToCommandStdin verifies that Copy passes the text via the
// returned command's Stdin. We stub the command builder to return /bin/cat,
// which echoes stdin -> stdout, and assert the round-trip.
func TestCopyPipesTextToCommandStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses /bin/cat")
	}

	var captured bytes.Buffer
	prev := command
	command = func() (*exec.Cmd, error) {
		cmd := exec.Command("/bin/cat")
		cmd.Stdout = &captured
		return cmd, nil
	}
	t.Cleanup(func() { command = prev })

	if err := Copy("hello world"); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	if got := captured.String(); got != "hello world" {
		t.Errorf("Copy should pipe text into the command's stdin; got %q", got)
	}
}
