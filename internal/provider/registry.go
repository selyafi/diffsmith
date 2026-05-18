package provider

import "fmt"

// Registry dispatches a URL to the first Provider that supports it.
//
// The order of providers passed to NewRegistry is the dispatch order; for
// v1 the order doesn't matter because GitHub and GitLab URLs don't overlap,
// but stable ordering keeps error messages deterministic.
type Registry struct {
	providers []Provider
}

// NewRegistry builds a Registry from a list of providers.
func NewRegistry(ps ...Provider) *Registry {
	return &Registry{providers: ps}
}

// Find returns the first provider that supports the URL, or an error
// listing the URL when nothing matches.
func (r *Registry) Find(rawURL string) (Provider, error) {
	for _, p := range r.providers {
		if p.Supports(rawURL) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no provider supports URL: %s", rawURL)
}
