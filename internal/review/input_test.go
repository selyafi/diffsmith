package review

import "testing"


// TestReviewTarget_Hostname: the capture-time host derived from the
// target URL, used to pin glab/gh calls to the instance the PR/MR
// actually lives on (diffsmith-1bk). Empty when the URL is unparseable
// so callers can fall back to the CLI's default host resolution.
func TestReviewTarget_Hostname(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://gitlab.example.com/group/proj/-/merge_requests/3", "gitlab.example.com"},
		{"https://gitlab.com/o/r/-/merge_requests/9", "gitlab.com"},
		{"://not-a-url", ""},
	}
	for _, c := range cases {
		tgt := ReviewTarget{URL: c.url}
		if got := tgt.Hostname(); got != c.want {
			t.Errorf("Hostname(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
