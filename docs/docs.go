// Package docs embeds documentation that CodeDiver serves at runtime, so
// agents read the version that matches the running build.
package docs

import _ "embed"

// Languages is LANGUAGES.md: per-language analysis precision and known gaps.
//
//go:embed LANGUAGES.md
var Languages string
