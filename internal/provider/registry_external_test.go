package provider_test

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/provider/githubgh"
	"github.com/selyafi/diffsmith/internal/provider/gitlabglab"
)

func TestRegistryByHost(t *testing.T) {
	// Reuse the real adapter types so we exercise the actual Supports
	// methods, not a hand-rolled fake.
	r := provider.NewRegistry(githubgh.New(nil), gitlabglab.New(nil))

	t.Run("github.com routes to githubgh", func(t *testing.T) {
		p, err := r.ByHost("github.com")
		if err != nil {
			t.Fatalf("ByHost: %v", err)
		}
		if _, ok := p.(*githubgh.Adapter); !ok {
			t.Errorf("got %T, want *githubgh.Adapter", p)
		}
	})

	t.Run("gitlab.com routes to gitlabglab", func(t *testing.T) {
		p, err := r.ByHost("gitlab.com")
		if err != nil {
			t.Fatalf("ByHost: %v", err)
		}
		if _, ok := p.(*gitlabglab.Adapter); !ok {
			t.Errorf("got %T, want *gitlabglab.Adapter", p)
		}
	})

	t.Run("unsupported host errors", func(t *testing.T) {
		if _, err := r.ByHost("bitbucket.org"); err == nil {
			t.Fatal("expected error for unsupported host")
		}
	})

	t.Run("empty host errors", func(t *testing.T) {
		if _, err := r.ByHost(""); err == nil {
			t.Fatal("expected error for empty host")
		}
	})
}
