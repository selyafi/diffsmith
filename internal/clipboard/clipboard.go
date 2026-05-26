package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// command builds the platform-specific clipboard command. Overridable in
// tests so behavior can be verified without touching a real clipboard.
var command = defaultCommand

func defaultCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "linux":
		// Probe both. The previous version returned xclip blindly when
		// wl-copy was missing, which on a Wayland machine without xclip
		// installed produced a confusing exec error mid-keystroke.
		// Now: if neither is on PATH, fail loudly with the install hint.
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command("wl-copy"), nil
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return exec.Command("xclip", "-selection", "clipboard"), nil
		}
		return nil, fmt.Errorf("clipboard: install wl-copy (Wayland) or xclip (X11) — neither is on PATH")
	case "windows":
		return exec.Command("clip"), nil
	default:
		return nil, fmt.Errorf("clipboard: unsupported platform %s", runtime.GOOS)
	}
}

// Copy writes text to the OS clipboard via the platform-specific helper,
// passing the payload through stdin (argv only — text is never expanded by a
// shell). Returns the helper's exit error on failure.
func Copy(text string) error {
	cmd, err := command()
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
