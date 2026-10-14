package commands

import "regexp"

var diagnosticSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/\s@]+@`),
	regexp.MustCompile(`(?i)([?&](?:access_token|token|password|secret|api_key|key|credential|signature|sig|x-amz-signature|x-amz-credential|x-amz-security-token)=)[^&\s"'<>]+`),
	regexp.MustCompile(`(?im)((?:(?:proxy-)?authorization|x-api-key|api-key|x-auth-token|private-token|x-gitlab-token)\s*:\s*)[^\r\n]+`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*)"(?:\\.|[^"\\])*"`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*)'(?:\\.|[^'\\])*'`),
	regexp.MustCompile(`(?i)((?:password|token|secret|api[_-]?key|credential)["']?\s*[:=]\s*["']?)[^\s,;"'}]+`),
	regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)`),
}

func redactDiagnosticText(text string) string {
	for i, pattern := range diagnosticSecretPatterns {
		replacement := "${1}[redacted]"
		if i == 0 {
			replacement += "@"
		} else if i == len(diagnosticSecretPatterns)-1 {
			replacement = "[redacted]"
		}

		text = pattern.ReplaceAllString(text, replacement)
	}

	return text
}
