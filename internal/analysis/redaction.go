package analysis

import "regexp"

var redactionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)(token|password|secret|api[_-]?key)(\s*[:=]\s*)([^\s"']+)`),
	regexp.MustCompile(`(?i)authorization:\s*bearer\s+[a-z0-9._-]+`),
}

func RedactSecrets(raw string) string {
	redacted := raw
	for _, pattern := range redactionPatterns {
		redacted = pattern.ReplaceAllStringFunc(redacted, func(match string) string {
			sub := pattern.FindStringSubmatch(match)
			if len(sub) == 4 {
				return sub[1] + sub[2] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return redacted
}
