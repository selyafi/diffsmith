package provider

import (
	"context"

	"github.com/selyafi/diffsmith/internal/review"
)

// Compile-time assertions that adapters implement the full Provider
// interface, including the new List/PreflightList methods.
var _ Provider = (*githubghAdapterAssert)(nil) // populated by adapter packages via blank import; see adapter tests

type githubghAdapterAssert struct{}

func (githubghAdapterAssert) Supports(string) bool                                       { return false }
func (githubghAdapterAssert) Preflight(context.Context) error                            { return nil }
func (githubghAdapterAssert) Fetch(context.Context, string) (*review.ReviewInput, error) { return nil, nil }
func (githubghAdapterAssert) PreflightList(context.Context) error                        { return nil }
func (githubghAdapterAssert) List(context.Context, RepoCoord) ([]PRSummary, error)       { return nil, nil }
