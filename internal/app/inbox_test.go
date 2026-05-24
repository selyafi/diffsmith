package app

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// fakeProvider lets us drive runInbox without real gh/glab calls.
type fakeProvider struct {
	host             string
	preflightListErr error
	listResp         []provider.PRSummary
	listErr          error
	listCalls        int
}

func (f *fakeProvider) Supports(rawURL string) bool                                      { return true }
func (f *fakeProvider) Preflight(ctx context.Context) error                              { return nil }
func (f *fakeProvider) Fetch(ctx context.Context, _ string) (*review.ReviewInput, error) { return nil, nil }
func (f *fakeProvider) PreflightList(ctx context.Context) error                          { return f.preflightListErr }
func (f *fakeProvider) List(ctx context.Context, _ provider.RepoCoord) ([]provider.PRSummary, error) {
	f.listCalls++
	return f.listResp, f.listErr
}

func TestRunInbox_PreflightFailureSurfaces(t *testing.T) {
	fp := &fakeProvider{host: "github.com", preflightListErr: errors.New("not authed")}
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	err := runInboxWithDeps(cmd, fp, provider.RepoCoord{Host: "github.com", Owner: "o", Name: "r"},
		func() (*provider.PRSummary, inboxAction, error) { return nil, inboxActionQuit, nil },
		func(ctx context.Context, cmd *cobra.Command, url string) error { return nil })

	if err == nil || err.Error() != "not authed" {
		t.Fatalf("expected 'not authed' propagation; got %v", err)
	}
}

func TestRunInbox_QuitImmediately(t *testing.T) {
	fp := &fakeProvider{listResp: []provider.PRSummary{{Number: 1}}}
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	err := runInboxWithDeps(cmd, fp, provider.RepoCoord{},
		func() (*provider.PRSummary, inboxAction, error) { return nil, inboxActionQuit, nil },
		func(ctx context.Context, cmd *cobra.Command, url string) error { return nil })

	if err != nil {
		t.Fatalf("expected nil err on clean quit; got %v", err)
	}
	if fp.listCalls != 1 {
		t.Errorf("expected exactly 1 List call (initial), got %d", fp.listCalls)
	}
}

func TestRunInbox_OpenThenQuit(t *testing.T) {
	fp := &fakeProvider{listResp: []provider.PRSummary{{Number: 42, URL: "https://example/42"}}}
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	calls := []string{}
	runs := []struct {
		action inboxAction
		pick   *provider.PRSummary
	}{
		{inboxActionOpen, &provider.PRSummary{URL: "https://example/42"}},
		{inboxActionQuit, nil},
	}
	idx := 0

	err := runInboxWithDeps(cmd, fp, provider.RepoCoord{},
		func() (*provider.PRSummary, inboxAction, error) {
			r := runs[idx]
			idx++
			return r.pick, r.action, nil
		},
		func(ctx context.Context, cmd *cobra.Command, url string) error {
			calls = append(calls, url)
			return nil
		})

	if err != nil {
		t.Fatalf("expected clean exit; got %v", err)
	}
	if len(calls) != 1 || calls[0] != "https://example/42" {
		t.Errorf("expected one runReview call to https://example/42; got %v", calls)
	}
}

func TestRunInbox_RefreshFetchesAgain(t *testing.T) {
	fp := &fakeProvider{listResp: []provider.PRSummary{{Number: 1}}}
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	runs := []inboxAction{inboxActionRefresh, inboxActionQuit}
	idx := 0
	err := runInboxWithDeps(cmd, fp, provider.RepoCoord{},
		func() (*provider.PRSummary, inboxAction, error) {
			r := runs[idx]
			idx++
			return nil, r, nil
		},
		func(ctx context.Context, cmd *cobra.Command, url string) error { return nil })

	if err != nil {
		t.Fatalf("clean exit expected; got %v", err)
	}
	if fp.listCalls != 2 {
		t.Errorf("expected 2 List calls (initial + refresh), got %d", fp.listCalls)
	}
}
