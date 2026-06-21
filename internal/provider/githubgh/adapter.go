package githubgh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// Adapter fetches GitHub pull request data via the `gh` CLI.
type Adapter struct {
	run       provider.Runner
	preflight *Preflight
	// warn receives human-readable notices about non-fatal fallback
	// behavior (e.g. files-API fallback engaged, oversize files skipped).
	// Defaults to os.Stderr; tests can override.
	warn io.Writer
}

// New constructs an Adapter. Passing nil uses provider.DefaultRunner and a
// default Preflight that calls exec.LookPath. Tests that need to substitute
// LookPath can build an Adapter literal directly.
func New(run provider.Runner) *Adapter {
	if run == nil {
		run = provider.DefaultRunner
	}
	return &Adapter{
		run:       run,
		preflight: NewPreflight(run, nil),
		warn:      os.Stderr,
	}
}

// Supports reports whether the URL is a GitHub pull request URL.
func (a *Adapter) Supports(rawURL string) bool {
	return Supports(rawURL)
}

// Preflight verifies gh is on PATH and authenticated before any fetch.
// New always wires a.preflight, and Adapter's fields are unexported so
// no zero-value literal can escape the package — a defensive nil-init
// here would be dead code.
func (a *Adapter) Preflight(ctx context.Context) error {
	return a.preflight.Check(ctx)
}

// Fetch retrieves PR metadata and the unified diff, then returns a
// normalized ReviewInput. The diff is parsed via internal/diff to surface
// classification errors early, before the model is invoked.
func (a *Adapter) Fetch(ctx context.Context, rawURL string) (*review.ReviewInput, error) {
	ref, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}

	meta, err := a.fetchMetadata(ctx, ref.URL)
	if err != nil {
		return nil, err
	}

	// No --patch: that returns the per-commit format-patch mailbox
	// series, where a file edited in two commits appears twice with
	// line numbers relative to each intermediate tree — diff.NewIndex
	// keeps only the last section, so findings can validate against
	// (and post to) the wrong lines. The default response is the
	// consolidated PR diff. diffsmith-a4o.
	rawDiff, err := a.run(ctx, nil, "gh", "pr", "diff", ref.URL, "--color", "never")
	if err != nil {
		if !isDiffTooLargeErr(err) {
			return nil, fmt.Errorf("gh pr diff: %w", err)
		}
		// GitHub server-side cap: the unified-diff endpoint refuses any
		// PR over 20,000 lines with HTTP 406. Reassemble from the files
		// API instead — it paginates and has only a per-file size cap.
		fmt.Fprintf(a.warn, "diffsmith: PR diff exceeds the GitHub 20K-line cap; falling back to the files API for %s\n", ref.URL)
		reassembled, ferr := a.fetchDiffViaFilesAPI(ctx, ref)
		if ferr != nil {
			return nil, ferr
		}
		rawDiff = []byte(reassembled)
	}
	files, err := diff.Parse(string(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	return &review.ReviewInput{
		Target: review.ReviewTarget{
			Host:    review.HostGitHub,
			URL:     firstNonEmpty(meta.URL, ref.URL),
			Owner:   ref.Owner,
			Repo:    ref.Repo,
			Number:  ref.Number,
			HeadRef: meta.HeadRefName,
			HeadSHA: meta.HeadRefOid,
			BaseRef: meta.BaseRefName,
		},
		Title:       meta.Title,
		Author:      meta.Author.Login,
		Description: meta.Body,
		Files:       files,
		RawDiff:     string(rawDiff),
	}, nil
}

// ghClosingRefs mirrors `gh pr view --json closingIssuesReferences`. The
// refs carry only number/url/repository (no title/body — see the
// gh-closing-issues-refs-shape note), so each issue body is fetched
// separately by FetchLinkedIssues.
type ghClosingRefs struct {
	ClosingIssuesReferences []struct {
		Number     int    `json:"number"`
		URL        string `json:"url"`
		Repository struct {
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	} `json:"closingIssuesReferences"`
}

// ghIssue mirrors `gh issue view <n> --json number,title,body,url`.
type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// FetchLinkedIssues resolves the issues this PR formally closes
// (closingIssuesReferences) and reads each one's title/body via
// `gh issue view`. diffsmith-144.
//
// Failure contract (review.LinkedIssueFetcher): a failure of the
// closing-refs query is total (returned as err — the caller surfaces it
// as one note and proceeds with no criteria); a failure on an individual
// issue is non-fatal (the issue is dropped and a note is appended), so one
// inaccessible cross-repo issue can't sink the rest.
func (a *Adapter) FetchLinkedIssues(ctx context.Context, target review.ReviewTarget) ([]review.IssueContext, []string, error) {
	out, err := a.run(ctx, nil, "gh", "pr", "view", target.URL, "--json", "closingIssuesReferences")
	if err != nil {
		return nil, nil, fmt.Errorf("gh pr view closingIssuesReferences: %w", err)
	}
	var refs ghClosingRefs
	if err := json.Unmarshal(out, &refs); err != nil {
		return nil, nil, fmt.Errorf("decode closingIssuesReferences JSON: %w", err)
	}

	var issues []review.IssueContext
	var notes []string
	if n := len(refs.ClosingIssuesReferences); n > review.MaxLinkedIssues {
		notes = append(notes, fmt.Sprintf("%d closing issue(s) beyond the first %d not fetched", n-review.MaxLinkedIssues, review.MaxLinkedIssues))
		refs.ClosingIssuesReferences = refs.ClosingIssuesReferences[:review.MaxLinkedIssues]
	}
	for _, r := range refs.ClosingIssuesReferences {
		owner, name := r.Repository.Owner.Login, r.Repository.Name
		if owner == "" {
			owner = target.Owner
		}
		if name == "" {
			name = target.Repo
		}
		repo := owner + "/" + name

		iout, ierr := a.run(ctx, nil, "gh", "issue", "view", strconv.Itoa(r.Number), "--repo", repo, "--json", "number,title,body,url")
		if ierr != nil {
			notes = append(notes, fmt.Sprintf("linked issue %s#%d: fetch failed: %v", repo, r.Number, ierr))
			continue
		}
		var iss ghIssue
		if jerr := json.Unmarshal(iout, &iss); jerr != nil {
			notes = append(notes, fmt.Sprintf("linked issue %s#%d: decode failed: %v", repo, r.Number, jerr))
			continue
		}
		issues = append(issues, review.IssueContext{
			Number: iss.Number,
			Title:  iss.Title,
			Body:   iss.Body,
			URL:    iss.URL,
		})
	}
	return issues, notes, nil
}

// ghMetadata mirrors the JSON shape returned by `gh pr view --json …`.
// HeadRefOid is the head commit SHA captured at diff-fetch time so the
// poster can anchor inline comments without re-resolving HEAD later.
type ghMetadata struct {
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
}

func (a *Adapter) fetchMetadata(ctx context.Context, prURL string) (*ghMetadata, error) {
	out, err := a.run(ctx, nil, "gh", "pr", "view", prURL, "--json", "title,author,body,headRefName,headRefOid,baseRefName,url")
	if err != nil {
		return nil, fmt.Errorf("gh pr view: %w", err)
	}
	var m ghMetadata
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("decode gh pr view JSON: %w", err)
	}
	return &m, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// isDiffTooLargeErr reports whether a `gh pr diff` failure is the
// GitHub 20,000-line server-side cap. gh surfaces the response body
// verbatim, so we match on both the HTTP status and the body phrase
// (either alone could match an unrelated 406; together they're tight).
func isDiffTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 406") && strings.Contains(msg, "exceeded the maximum number of lines")
}

// ghPullFile is one entry of the response from GET /repos/{o}/{r}/pulls/{N}/files.
// We only need the fields used to reconstruct a unified diff; the API
// returns more (sha, blob_url, additions, …) which json.Unmarshal will
// discard silently.
type ghPullFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	// Changes (additions+deletions) disambiguates WHY Patch is null:
	// zero means there is no text diff to show (binary file, pure
	// rename, mode-only change) and the file should surface as a
	// metadata-only segment; non-zero means the patch exists but
	// exceeds GitHub's per-file cap. diffsmith-oks.
	Changes int `json:"changes"`
	// Patch is the file's unified-diff body (hunks only — no
	// `--- a/...` / `+++ b/...` headers). Null when the file's patch
	// exceeds the per-file ~3MB cap, or when there is no text diff at
	// all (see Changes).
	Patch *string `json:"patch"`
}

// fetchDiffViaFilesAPI calls `gh api repos/{o}/{r}/pulls/{N}/files
// --paginate` and reassembles a unified diff string from the per-file
// patches. Files whose patch field is null (per-file cap) are warned
// about on stderr and skipped — better to review the rest than abort.
func (a *Adapter) fetchDiffViaFilesAPI(ctx context.Context, ref *PullRequestRef) (string, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d/files", ref.Owner, ref.Repo, ref.Number)
	out, err := a.run(ctx, nil, "gh", "api", apiPath, "--paginate")
	if err != nil {
		return "", fmt.Errorf("gh api %s: %w", apiPath, err)
	}
	// --paginate emits one JSON array per page (and >20K-line PRs
	// usually have >100 files, i.e. multiple pages); DecodePages
	// handles single- and multi-page output alike. diffsmith-kjk.
	files, err := provider.DecodePages[ghPullFile](out)
	if err != nil {
		preview := string(out)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return "", fmt.Errorf("decode gh api files JSON: %w (raw: %s)", err, preview)
	}
	var b strings.Builder
	skipped := 0
	for _, f := range files {
		if f.Patch == nil {
			// No patch + no changes = nothing to diff (binary, pure
			// rename, mode-only): synthesize a metadata-only segment so
			// the file reaches the prompt's # Files section like it does
			// on the normal `gh pr diff` path. Only a non-zero change
			// count means a genuinely size-capped text patch.
			// diffsmith-oks.
			if f.Changes == 0 {
				writeMetadataOnlyFile(&b, f)
				continue
			}
			skipped++
			fmt.Fprintf(a.warn, "diffsmith: skipping %s (patch exceeds GitHub per-file size cap; not reviewable via files API)\n", f.Filename)
			continue
		}
		writeReassembledFile(&b, f)
	}
	if skipped > 0 {
		fmt.Fprintf(a.warn, "diffsmith: %d file(s) skipped during files-API fallback\n", skipped)
	}
	return b.String(), nil
}

// writeMetadataOnlyFile emits a hunk-less diff segment for a file with
// no text diff (binary, pure rename, mode-only), mirroring git's own
// headers so diff.Parse classifies it FilePureRename / FileBinary and
// the prompt lists it as metadata-only. Mode-only changes are not
// distinguishable from binary in the files-API response (both have
// changes==0, no patch); they land under the binary header, which still
// renders as metadata-only. diffsmith-oks.
func writeMetadataOnlyFile(b *strings.Builder, f ghPullFile) {
	oldPath := f.Filename
	if f.PreviousFilename != "" {
		oldPath = f.PreviousFilename
	}
	fmt.Fprintf(b, "diff --git a/%s b/%s\n", oldPath, f.Filename)
	switch f.Status {
	case "renamed", "copied":
		b.WriteString("similarity index 100%\n")
		fmt.Fprintf(b, "rename from %s\nrename to %s\n", oldPath, f.Filename)
	default:
		// The placeholder index line is required for go-diff to resolve
		// the file names of a binary segment; its SHAs are never read.
		b.WriteString("index 0000000..0000000 100644\n")
		fmt.Fprintf(b, "Binary files a/%s and b/%s differ\n", oldPath, f.Filename)
	}
}

// writeReassembledFile emits one file's section of a unified diff:
// the `diff --git` header, the appropriate `---`/`+++` paths for the
// file's status, and the patch body returned by the files API.
//
// Path mapping by status:
//   - added            → ---/dev/null  +++ b/<new>
//   - removed          → --- a/<new>   +++ /dev/null
//   - renamed | copied → --- a/<previous>  +++ b/<new>
//   - default          → --- a/<new>   +++ b/<new>
//
// The `index <oldsha>..<newsha>` line is omitted: the files API doesn't
// expose the pre-image blob SHA, and sourcegraph/go-diff doesn't require
// it. classify() only inspects extended headers we still emit for
// added/removed/renamed paths.
func writeReassembledFile(b *strings.Builder, f ghPullFile) {
	oldPath := f.Filename
	if f.PreviousFilename != "" {
		oldPath = f.PreviousFilename
	}
	fmt.Fprintf(b, "diff --git a/%s b/%s\n", oldPath, f.Filename)
	switch f.Status {
	case "added":
		b.WriteString("new file mode 100644\n")
		fmt.Fprintf(b, "--- /dev/null\n+++ b/%s\n", f.Filename)
	case "removed":
		b.WriteString("deleted file mode 100644\n")
		fmt.Fprintf(b, "--- a/%s\n+++ /dev/null\n", f.Filename)
	case "renamed", "copied":
		fmt.Fprintf(b, "rename from %s\nrename to %s\n", oldPath, f.Filename)
		fmt.Fprintf(b, "--- a/%s\n+++ b/%s\n", oldPath, f.Filename)
	default:
		fmt.Fprintf(b, "--- a/%s\n+++ b/%s\n", f.Filename, f.Filename)
	}
	b.WriteString(*f.Patch)
	if !strings.HasSuffix(*f.Patch, "\n") {
		b.WriteByte('\n')
	}
}

// Compile-time guard: the GitHub adapter provides acceptance-criteria
// enrichment (diffsmith-144). The app type-asserts to this capability.
var _ review.LinkedIssueFetcher = (*Adapter)(nil)

// PreflightList verifies gh is authenticated before listing PRs.
func (a *Adapter) PreflightList(ctx context.Context) error {
	if _, err := a.run(ctx, nil, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is installed but not authenticated; run 'gh auth login': %w", err)
	}
	return nil
}

type ghAuthor struct {
	Login    string `json:"login"`
	TypeName string `json:"__typename"`
}

type ghPRNode struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	IsDraft   bool      `json:"isDraft"`
	Author    ghAuthor  `json:"author"`
	Comments  struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ReviewThreads struct {
		Nodes []struct {
			IsResolved bool `json:"isResolved"`
			Comments   struct {
				TotalCount int `json:"totalCount"`
				Nodes      []struct {
					Author ghAuthor `json:"author"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
	Reviews struct {
		Nodes []struct {
			Author ghAuthor `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`
}

const ghListQuery = `query($q:String!){
  search(query:$q, type:ISSUE, first:30){
    nodes{ ... on PullRequest {
      number title url updatedAt isDraft
      author{ login __typename }
      comments{ totalCount }
      reviewThreads(first:100){ nodes{
        isResolved
        comments(first:100){ totalCount nodes{ author{ login __typename } } }
      } }
      reviews(first:100){ nodes{ author{ login __typename } } }
    } }
  }
}`

// List enumerates open PRs for the repo and enriches each row with comment
// count, resolved/unresolved thread counts, and human commenters, all in a
// single GraphQL call.
func (a *Adapter) List(ctx context.Context, repo provider.RepoCoord) ([]provider.PRSummary, error) {
	search := fmt.Sprintf("repo:%s/%s is:pr is:open", repo.Owner, repo.Name)
	out, err := a.run(ctx, nil, "gh", "api", "graphql", "-f", "query="+ghListQuery, "-f", "q="+search)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql (pr list): %w", err)
	}
	var resp struct {
		Data struct {
			Search struct {
				Nodes []ghPRNode `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		preview := string(out)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return nil, fmt.Errorf("failed to parse gh graphql output: %w (raw: %s)", err, preview)
	}
	result := make([]provider.PRSummary, 0, len(resp.Data.Search.Nodes))
	for _, n := range resp.Data.Search.Nodes {
		s := provider.PRSummary{
			Number: n.Number, Title: n.Title, Author: n.Author.Login,
			URL: n.URL, UpdatedAt: n.UpdatedAt, Draft: n.IsDraft, Enriched: true,
		}
		s.CommentCount = n.Comments.TotalCount
		humans := newHumanSet(n.Author.Login)
		for _, t := range n.ReviewThreads.Nodes {
			if t.IsResolved {
				s.ResolvedThreads++
			} else {
				s.UnresolvedThreads++
			}
			s.CommentCount += t.Comments.TotalCount
			for _, c := range t.Comments.Nodes {
				humans.add(c.Author)
			}
		}
		for _, r := range n.Reviews.Nodes {
			humans.add(r.Author)
		}
		s.HumanCommenters = humans.list()
		result = append(result, s)
	}
	return result, nil
}

// humanSet collects distinct human commenter logins in first-seen order,
// excluding bots (GraphQL __typename or IsBotLogin fallback) and the PR author.
type humanSet struct {
	author string
	seen   map[string]bool
	order  []string
}

func newHumanSet(author string) *humanSet {
	return &humanSet{author: author, seen: map[string]bool{}}
}

func (h *humanSet) add(a ghAuthor) {
	if a.Login == "" || a.Login == h.author || a.TypeName == "Bot" || provider.IsBotLogin(a.Login) {
		return
	}
	if !h.seen[a.Login] {
		h.seen[a.Login] = true
		h.order = append(h.order, a.Login)
	}
}

func (h *humanSet) list() []string { return h.order }
