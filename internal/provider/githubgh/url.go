package githubgh

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// PullRequestRef identifies a GitHub pull request parsed from a URL.
type PullRequestRef struct {
	Owner  string
	Repo   string
	Number int
	URL    string // canonical form: https://github.com/<owner>/<repo>/pull/<number>
}

// ParseURL extracts a PullRequestRef from a GitHub pull request URL.
//
// Accepts the canonical form and any sub-path under it (e.g. /files,
// /commits), query strings, and trailing slashes. Rejects URLs that are not
// pull requests, not GitHub, or not HTTPS.
func ParseURL(raw string) (*PullRequestRef, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("github URL must use https scheme")
	}
	if u.Host != "github.com" {
		return nil, fmt.Errorf("unsupported host %q (expected github.com)", u.Host)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// Expect: <owner>/<repo>/pull/<number>[/...]
	if len(parts) < 4 || parts[2] != "pull" {
		return nil, fmt.Errorf("not a github pull request URL: %s", raw)
	}
	owner, repo, numStr := parts[0], parts[1], parts[3]
	if owner == "" || repo == "" || numStr == "" {
		return nil, fmt.Errorf("malformed github pull request URL: %s", raw)
	}
	number, err := strconv.Atoi(numStr)
	if err != nil || number <= 0 {
		return nil, fmt.Errorf("invalid PR number %q in URL", numStr)
	}

	return &PullRequestRef{
		Owner:  owner,
		Repo:   repo,
		Number: number,
		URL:    fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number),
	}, nil
}

// Supports reports whether this adapter can handle the URL. Used by the
// provider registry to dispatch.
func Supports(raw string) bool {
	_, err := ParseURL(raw)
	return err == nil
}
