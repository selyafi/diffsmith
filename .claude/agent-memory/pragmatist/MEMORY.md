# Memory Index

- [Recurring over-engineering patterns](patterns-over-engineering.md) — Common shapes of waste seen in this codebase: defensive nil re-init, getters never called, single-call helpers, future-milestone stub packages.
- [Load-bearing "over-engineering" not to flag](load-bearing-abstractions.md) — Abstractions that look heavy but earn their keep: stripWrapper, package-level seams (runGH, runTUI, etc.), antigravity Preflight stub.
