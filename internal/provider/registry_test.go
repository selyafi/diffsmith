package provider

import (
	"context"
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

type stubProvider struct {
	name     string
	supports func(string) bool
}

func (s *stubProvider) Supports(u string) bool {
	return s.supports(u)
}

func (s *stubProvider) Preflight(context.Context) error {
	return nil
}

func (s *stubProvider) Fetch(context.Context, string) (*review.ReviewInput, error) {
	return nil, nil
}

func TestRegistryFind(t *testing.T) {
	gh := &stubProvider{name: "github", supports: func(u string) bool { return u == "gh-url" }}
	gl := &stubProvider{name: "gitlab", supports: func(u string) bool { return u == "gl-url" }}
	r := NewRegistry(gh, gl)

	if p, err := r.Find("gh-url"); err != nil || p != gh {
		t.Errorf("gh-url: got (%v, %v), want gh", p, err)
	}
	if p, err := r.Find("gl-url"); err != nil || p != gl {
		t.Errorf("gl-url: got (%v, %v), want gl", p, err)
	}
	if _, err := r.Find("nope"); err == nil {
		t.Error("nope: want error, got nil")
	}
}
