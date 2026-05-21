package gitlabglab

import "testing"

func TestParseURL(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		projectPath string
		number      int
		repoURL     string
		wantErr     bool
	}{
		{
			name:        "canonical single-group MR URL",
			input:       "https://gitlab.com/group/project/-/merge_requests/123",
			projectPath: "group/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/project",
		},
		{
			name:        "nested-group MR URL (one subgroup)",
			input:       "https://gitlab.com/group/subgroup/project/-/merge_requests/123",
			projectPath: "group/subgroup/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/subgroup/project",
		},
		{
			name:        "deeply nested groups",
			input:       "https://gitlab.com/a/b/c/project/-/merge_requests/9001",
			projectPath: "a/b/c/project",
			number:      9001,
			repoURL:     "https://gitlab.com/a/b/c/project",
		},
		{
			name:        "trailing slash",
			input:       "https://gitlab.com/group/project/-/merge_requests/123/",
			projectPath: "group/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/project",
		},
		{
			name:        "diffs sub-path",
			input:       "https://gitlab.com/group/project/-/merge_requests/123/diffs",
			projectPath: "group/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/project",
		},
		{
			name:        "commits sub-path",
			input:       "https://gitlab.com/group/project/-/merge_requests/123/commits",
			projectPath: "group/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/project",
		},
		{
			name:        "with query string",
			input:       "https://gitlab.com/group/project/-/merge_requests/123?tab=diffs",
			projectPath: "group/project",
			number:      123,
			repoURL:     "https://gitlab.com/group/project",
		},
		{
			name:        "hyphens and dots in path segments",
			input:       "https://gitlab.com/some-group/my.project-v2/-/merge_requests/9001",
			projectPath: "some-group/my.project-v2",
			number:      9001,
			repoURL:     "https://gitlab.com/some-group/my.project-v2",
		},
		{
			name:    "GitHub URL rejected (host guard)",
			input:   "https://github.com/owner/repo/pull/123",
			wantErr: true,
		},
		{
			name:    "self-hosted GitLab rejected in V1 (host guard)",
			input:   "https://gitlab.example.com/group/project/-/merge_requests/123",
			wantErr: true,
		},
		{
			name:    "non-https scheme",
			input:   "http://gitlab.com/group/project/-/merge_requests/123",
			wantErr: true,
		},
		{
			name:    "issue URL is not an MR",
			input:   "https://gitlab.com/group/project/-/issues/123",
			wantErr: true,
		},
		{
			name:    "missing /-/merge_requests/ segment",
			input:   "https://gitlab.com/group/project/123",
			wantErr: true,
		},
		{
			name:    "missing MR number",
			input:   "https://gitlab.com/group/project/-/merge_requests/",
			wantErr: true,
		},
		{
			name:    "non-numeric MR number",
			input:   "https://gitlab.com/group/project/-/merge_requests/abc",
			wantErr: true,
		},
		{
			name:    "zero MR number",
			input:   "https://gitlab.com/group/project/-/merge_requests/0",
			wantErr: true,
		},
		{
			name:    "single-segment path (no project)",
			input:   "https://gitlab.com/group/-/merge_requests/123",
			wantErr: true,
		},
		{
			name:    "garbage input",
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
			if got.ProjectPath != tc.projectPath {
				t.Errorf("ProjectPath: got %q, want %q", got.ProjectPath, tc.projectPath)
			}
			if got.Number != tc.number {
				t.Errorf("Number: got %d, want %d", got.Number, tc.number)
			}
			if got.RepoURL != tc.repoURL {
				t.Errorf("RepoURL: got %q, want %q", got.RepoURL, tc.repoURL)
			}
			// URL should be the canonical form: repoURL + /-/merge_requests/<number>
			wantURL := tc.repoURL + "/-/merge_requests/" + itoaForTest(tc.number)
			if got.URL != wantURL {
				t.Errorf("URL: got %q, want %q", got.URL, wantURL)
			}
		})
	}
}

func TestSupports(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"https://gitlab.com/group/project/-/merge_requests/123", true},
		{"https://gitlab.com/group/subgroup/project/-/merge_requests/123", true},
		{"https://gitlab.com/group/project/-/merge_requests/123/diffs", true},
		{"https://gitlab.com/group/project/-/issues/123", false},
		{"https://github.com/owner/repo/pull/123", false},
		{"https://gitlab.example.com/group/project/-/merge_requests/123", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := Supports(tc.input); got != tc.want {
			t.Errorf("Supports(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// itoaForTest is a tiny local stringer used only to build the expected URL
// inside the table test, keeping the test file dependency-free.
func itoaForTest(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
