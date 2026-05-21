package gitlabglab

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MergeRequestRef identifies a GitLab merge request parsed from a URL.
// ProjectPath is the full URL-path namespace+project (e.g. "group/project"
// or "group/sub/project" for nested groups); the adapter splits it at the
// last slash to populate review.ReviewTarget.Owner/Repo. RepoURL is the
// project URL passed to `glab -R`.
type MergeRequestRef struct {
	ProjectPath string
	Number      int
	URL         string // canonical: https://gitlab.com/<project-path>/-/merge_requests/<number>
	RepoURL     string // https://gitlab.com/<project-path>
}

// mrSeparator is the literal segment GitLab uses to scope MR-level
// resources. The "/-/" prefix is reserved and cannot appear inside a
// project path, so finding this substring unambiguously splits the URL
// regardless of namespace depth.
const mrSeparator = "/-/merge_requests/"

// ParseURL extracts a MergeRequestRef from a GitLab merge request URL.
//
// Accepts the canonical form and any sub-path under it (e.g. /diffs,
// /commits), query strings, and trailing slashes. Supports nested
// groups of any depth (gitlab.com/<group>/<sub>/.../<project>). Rejects
// URLs that are not MRs, not gitlab.com, or not HTTPS — self-hosted
// GitLab is a V1.x concern, not V1.
func ParseURL(raw string) (*MergeRequestRef, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("gitlab URL must use https scheme")
	}
	if u.Host != "gitlab.com" {
		return nil, fmt.Errorf("unsupported host %q (expected gitlab.com)", u.Host)
	}

	idx := strings.Index(u.Path, mrSeparator)
	if idx < 0 {
		return nil, fmt.Errorf("not a gitlab merge request URL: %s", raw)
	}

	projectPath := strings.Trim(u.Path[:idx], "/")
	if !strings.Contains(projectPath, "/") {
		return nil, fmt.Errorf("malformed gitlab MR URL: project path must include namespace: %s", raw)
	}

	rest := strings.Trim(u.Path[idx+len(mrSeparator):], "/")
	idSegment, _, _ := strings.Cut(rest, "/")
	if idSegment == "" {
		return nil, fmt.Errorf("malformed gitlab MR URL: missing MR number: %s", raw)
	}
	number, err := strconv.Atoi(idSegment)
	if err != nil || number <= 0 {
		return nil, fmt.Errorf("invalid MR number %q in URL", idSegment)
	}

	repoURL := "https://gitlab.com/" + projectPath
	return &MergeRequestRef{
		ProjectPath: projectPath,
		Number:      number,
		URL:         repoURL + mrSeparator + strconv.Itoa(number),
		RepoURL:     repoURL,
	}, nil
}

// Supports reports whether this adapter can handle the URL. Used by the
// provider registry to dispatch.
func Supports(raw string) bool {
	_, err := ParseURL(raw)
	return err == nil
}
