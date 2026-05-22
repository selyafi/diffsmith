// Package antigravitycli implements the Antigravity model adapter
// (experimental in v1). The CLI binary is `agy`. Lands in M7.
//
// # Status (S8b spike, 2026-05-22)
//
// `agy --print` (alias `-p` / `--prompt`) is the non-interactive mode.
// Output is the raw model text with no envelope, so the adapter can pipe
// stdout directly to `model.ParseFindings` (unlike Gemini's `-o json`
// which wraps in `{"response": ...}`). Stdin is supported. There is no
// `--output-format json` flag, so JSON reliability is prompt-engineered
// — same risk profile as Codex without `--output-schema`.
//
// However, `agy` is gated behind interactive browser OAuth on every
// invocation. Each call without a live session prints a Google login
// URL and listens on `https://antigravity.google/oauth-callback` with
// a 30-second timeout. The tokens do not persist across invocations and
// the CLI does not share auth with the installed Antigravity desktop
// app (both use the same OAuth client_id but different redirect URIs).
//
// This makes `agy` unsuitable for non-interactive review in v1. The
// adapter therefore ships behind a Preflight error per the v1 plan
// (`model-adapters.md`, `implementation-plan.md` M7), and is excluded
// from the supported-models list in `--help` and the README until
// Antigravity provides a persistent-token or API-key auth path.
package antigravitycli
