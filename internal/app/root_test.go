package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpListsReviewSubcommand(t *testing.T) {
	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--help should not error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "review") {
		t.Fatalf("help output should advertise the review subcommand, got:\n%s", got)
	}
	if !strings.Contains(got, "diffsmith") {
		t.Fatalf("help output should mention the binary name, got:\n%s", got)
	}
}

func TestReviewRequiresURLArgument(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"review"})

	if err := root.Execute(); err == nil {
		t.Fatal("review without a URL argument should error")
	}
}
