package gitlabglab

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPreflightMissingBinary(t *testing.T) {
	p := NewPreflight(
		func(context.Context, io.Reader, string, ...string) ([]byte, error) {
			t.Fatal("runner must not be invoked when glab is missing from PATH")
			return nil, nil
		},
		func(string) (string, error) {
			return "", errors.New("exec: \"glab\": executable file not found in $PATH")
		},
	)

	err := p.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "glab CLI not found on PATH") {
		t.Errorf("want missing-binary error mentioning 'glab CLI not found on PATH', got: %v", err)
	}
}

func TestPreflightAuthFailure(t *testing.T) {
	transportErr := errors.New("glab: exit 1: not logged in")
	p := NewPreflight(
		func(context.Context, io.Reader, string, ...string) ([]byte, error) {
			return nil, transportErr
		},
		func(string) (string, error) { return "/opt/homebrew/bin/glab", nil },
	)

	err := p.Check(context.Background())
	if err == nil {
		t.Fatal("want auth-failure error, got nil")
	}
	// The full actionable substring must appear CONTIGUOUSLY — a weaker
	// 'glab auth login' substring check would pass a buggy ordering where
	// the underlying error splits the actionable text.
	const wantActionable = "glab is not authenticated. Run `glab auth login` to authenticate"
	if !strings.Contains(err.Error(), wantActionable) {
		t.Errorf("auth-failure error missing required contiguous actionable text\nWANT substring: %q\nGOT: %v", wantActionable, err)
	}
	// The wrap must preserve the underlying transport error via errors.Is.
	if !errors.Is(err, transportErr) {
		t.Errorf("auth-failure error must wrap the underlying transport error via %%w; errors.Is(err, transportErr) was false. Got: %v", err)
	}
}

func TestPreflightHappyPath(t *testing.T) {
	called := false
	p := NewPreflight(
		func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
			called = true
			if name != "glab" || len(args) < 2 || args[0] != "auth" || args[1] != "status" {
				t.Errorf("unexpected call: %s %v", name, args)
			}
			return []byte("Authenticated as alice"), nil
		},
		func(string) (string, error) { return "/opt/homebrew/bin/glab", nil },
	)

	if err := p.Check(context.Background()); err != nil {
		t.Errorf("want nil error on happy path, got: %v", err)
	}
	if !called {
		t.Error("runner was not invoked")
	}
}
