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

// probeURLs are synthetic PR/MR URLs used by ByHost to exercise each
// provider's Supports method. We include both a GitHub-style pull-request
// path and a GitLab-style merge-request path so that ByHost works for both
// adapters without adding a Host() method to the Provider interface.
var probeURLs = []string{
	"https://%s/owner/repo/pull/1",              // GitHub-style
	"https://%s/owner/repo/-/merge_requests/1",  // GitLab-style
}

// ByHost returns the provider that claims the given host, or an error
// if none does. Implementation: synthesize candidate PR/MR URLs and
// reuse each Provider's Supports method. Avoids adding a Host() method
// to the Provider interface just for this helper.
func (r *Registry) ByHost(host string) (Provider, error) {
	if host == "" {
		return nil, fmt.Errorf("provider: empty host")
	}
	for _, p := range r.providers {
		for _, tmpl := range probeURLs {
			probe := fmt.Sprintf(tmpl, host)
			if p.Supports(probe) {
				return p, nil
			}
		}
	}
	return nil, fmt.Errorf("provider: host %q not supported", host)
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
