// Package antigravitycli implements the Antigravity model adapter. The CLI
// binary is `agy`. It is a full peer of the codex/claude adapters —
// reviewer, synthesizer, and input-budget setter.
//
// # Invocation
//
// agy is driven non-interactively via:
//
//	agy --print=- --print-timeout <dur> --model <name>   (prompt via stdin)
//
// agy's `--print` is a string flag that REQUIRES a value (it is not a
// boolean toggle); `-` is the conventional stdin marker. agy is
// multi-model (Gemini, Claude, GPT-OSS variants — see `agy models`); the
// adapter pins `--model` to DefaultModel so reviews are reproducible
// rather than depending on agy's user/config session default. Override
// via SetModel (wired to --antigravity-model / $DIFFSMITH_ANTIGRAVITY_MODEL). When stdin is a
// pipe, agy reads the prompt from it, so prompts up to the 1 MiB input
// budget travel via stdin per ADR 0007 rather than argv (past ARG_MAX).
// Output is raw model text with no envelope, so stdout pipes directly into
// model.ParseFindings — unlike Gemini's `-o json`, which wrapped output in
// {"response": …}. There is no --output-schema flag, so JSON reliability
// is prompt-engineered (the same risk profile as codex without a schema),
// handled by the defensive parser.
//
// # Auth (S8b resolved)
//
// Spike S8b (2026-05-22) stubbed this adapter because agy v1.0.0 required
// interactive browser OAuth on every invocation. agy 1.0.9 fixed that
// ("Fixed OAuth token persistence and authentication hangs"), so tokens
// persist across calls and `agy --print` runs non-interactively once the
// user has logged in. Preflight only checks that `agy` is on PATH; a user
// who has never authenticated sees agy's own login prompt surfaced at
// Review time (the runner propagates stderr), matching codex/claude.
package antigravitycli
