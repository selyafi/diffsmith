package githubgh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPreflightMissingBinary(t *testing.T) {
	p := NewPreflight(
		func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("runner should not be invoked when gh is missing")
			return nil, nil
		},
		func(string) (string, error) {
			return "", errors.New("exec: \"gh\": executable file not found in $PATH")
		},
	)

	err := p.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gh CLI not found on PATH") {
		t.Errorf("want missing-binary error, got: %v", err)
	}
}

func TestPreflightAuthFailure(t *testing.T) {
	p := NewPreflight(
		func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("gh: exit 1: not logged in")
		},
		func(string) (string, error) { return "/usr/local/bin/gh", nil },
	)

	err := p.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("want auth-failure error suggesting `gh auth login`, got: %v", err)
	}
}

func TestPreflightHappyPath(t *testing.T) {
	called := false
	p := NewPreflight(
		func(_ context.Context, name string, args ...string) ([]byte, error) {
			called = true
			if name != "gh" || len(args) < 2 || args[0] != "auth" || args[1] != "status" {
				t.Errorf("unexpected call: %s %v", name, args)
			}
			return []byte("logged in as alice"), nil
		},
		func(string) (string, error) { return "/usr/local/bin/gh", nil },
	)

	if err := p.Check(context.Background()); err != nil {
		t.Errorf("want nil error, got: %v", err)
	}
	if !called {
		t.Error("runner was not invoked")
	}
}
