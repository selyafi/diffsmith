// Package geminicli implements the Gemini model adapter via
// `gemini -o text`. Prompts are piped via stdin per ADR 0007.
//
// # Why text mode, not json
//
// `gemini -o json` returns a structured envelope with response, stats,
// and error fields ({"response": "<model text>", ...}). Text mode
// returns the raw model output, which by prompt contract is the
// {"findings":[...]} JSON object that model.ParseFindings expects.
// Skipping the envelope keeps parsing identical across all adapters.
//
// # Why no --output-schema
//
// Gemini has no native schema flag (unlike Codex which uses
// --output-schema for runtime enforcement). JSON shape is enforced via
// prompt instructions and validated by model.ParseFindings, which
// already tolerates the kinds of drift gemini sometimes produces (code
// fences, leading prose). Same risk profile as the Claude adapter.
//
// # Replaces antigravity as default third model
//
// Antigravity (`agy`) requires interactive browser OAuth on every
// invocation with no persistent-token path (spike S8b), so it cannot
// run non-interactively. Gemini fills the third-model slot in v1 with
// the same priority pattern (codex > claude > gemini) used by the
// model picker and synthesis chain-fallback.
package geminicli
