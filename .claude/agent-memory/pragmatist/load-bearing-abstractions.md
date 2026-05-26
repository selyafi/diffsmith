---
name: load-bearing-abstractions
description: Abstractions in diffsmith that look heavy but earn their keep — don't flag these in audits.
metadata:
  type: project
---

Things that look like over-engineering in diffsmith but are deliberately load-bearing — do NOT flag in audits:

1. **`model.stripWrapper`** (`internal/model/parse.go`) — defensive parsing that peels markdown fences / prose preamble from model output. Looks like loose validation; product team confirmed it's load-bearing for real-world model drift (gemini fences, claude chatter).

2. **Package-level test seams** (`runGH`, `runGlab`, `submitPost`, `pickerRunner`, `runTUI`, `httpFetcher`, `clipboard.command`, `tui.copyToClipboard`) — every one has live test callers. They look like indirection but each is the cheapest way to keep the test suite hermetic without an interface refactor.

3. **`antigravitycli` package** — entirely a Preflight-error stub per spike S8b (interactive OAuth gate). The "useless" stub exists so `--model antigravity` produces an actionable error instead of "unknown model". Documented in `doc.go`; do not propose removal.

4. **Per-finding ParseError categories** (`Kind: "prose_preamble" | "invalid_json" | "wrong_shape"`) — three categories with one consumer looks like premature taxonomy, but each Kind drives different debug UX in the TUI.

5. **`runInboxWithDeps` wrapper over `runInbox(...nil...)`** — looks like a single-call wrapper but has four test call sites and the `nil` `modelPtr` arg has semantic meaning (`inbox.go:99` comment).

**Why:** Architect explicitly noted on 2026-05-25 that "defensive parsing was the RIGHT abstraction even though it loosens validation; some over-engineering is actually load-bearing for product behavior."

**How to apply:** Before flagging any of these as removable, re-read the comment at the declaration site. If the comment cites a spike (S8b, S9, M7a), an ADR, or specific product behavior, the abstraction is intentional. Flag only the unjustified ones.
