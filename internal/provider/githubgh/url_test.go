package githubgh

import "testing"

func TestParseURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		owner   string
		repo    string
		number  int
		wantErr bool
	}{
		{
			name:   "canonical PR URL",
			input:  "https://github.com/owner/repo/pull/123",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			name:   "trailing slash",
			input:  "https://github.com/owner/repo/pull/123/",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			name:   "PR files sub-path",
			input:  "https://github.com/owner/repo/pull/123/files",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			name:   "PR commits sub-path",
			input:  "https://github.com/owner/repo/pull/123/commits",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			name:   "with query string",
			input:  "https://github.com/owner/repo/pull/123?w=1",
			owner:  "owner",
			repo:   "repo",
			number: 123,
		},
		{
			name:   "hyphens and dots in repo",
			input:  "https://github.com/some-org/my.repo-v2/pull/9001",
			owner:  "some-org",
			repo:   "my.repo-v2",
			number: 9001,
		},
		{
			name:    "issue URL is not a PR",
			input:   "https://github.com/owner/repo/issues/123",
			wantErr: true,
		},
		{
			name:    "GitLab URL not supported here",
			input:   "https://gitlab.com/group/project/-/merge_requests/123",
			wantErr: true,
		},
		{
			name:    "non-https scheme",
			input:   "http://github.com/owner/repo/pull/123",
			wantErr: true,
		},
		{
			name:    "wrong host",
			input:   "https://example.com/owner/repo/pull/123",
			wantErr: true,
		},
		{
			name:    "missing PR number",
			input:   "https://github.com/owner/repo/pull/",
			wantErr: true,
		},
		{
			name:    "non-numeric PR number",
			input:   "https://github.com/owner/repo/pull/abc",
			wantErr: true,
		},
		{
			name:    "malformed",
			input:   "not a url",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (parsed: %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Owner != tc.owner {
				t.Errorf("Owner: got %q, want %q", got.Owner, tc.owner)
			}
			if got.Repo != tc.repo {
				t.Errorf("Repo: got %q, want %q", got.Repo, tc.repo)
			}
			if got.Number != tc.number {
				t.Errorf("Number: got %d, want %d", got.Number, tc.number)
			}
		})
	}
}

func TestSupports(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://github.com/owner/repo/pull/123", true},
		{"https://github.com/owner/repo/pull/123/files", true},
		{"https://github.com/owner/repo/issues/123", false},
		{"https://gitlab.com/group/project/-/merge_requests/123", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := Supports(tc.input); got != tc.want {
			t.Errorf("Supports(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
