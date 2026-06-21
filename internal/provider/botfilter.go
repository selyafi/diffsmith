package provider

import "strings"

// IsBotLogin reports whether a forge login belongs to an automated
// reviewer rather than a human. It is a best-effort heuristic used to keep
// the inbox's commenter list to humans (the "who's waiting on me" signal).
// GitHub callers should prefer the GraphQL __typename == "Bot" signal and
// use this only as a fallback; GitLab has no clean bot flag, so this is the
// primary filter there.
func IsBotLogin(login string) bool {
	l := strings.ToLower(login)
	if l == "" {
		return false
	}
	if strings.HasSuffix(l, "[bot]") || strings.HasSuffix(l, "-bot") || strings.HasSuffix(l, "_bot") {
		return true
	}
	for _, b := range []string{"copilot", "coderabbit", "greptile", "github-actions", "dependabot", "renovate"} {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
}
