package analysis

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	input := "token=abcd1234\nAuthorization: Bearer abc.def.ghi\nghp_1234567890123456789012\ngithub_pat_abcdefghijklmnopqrstuvwxyz123456"
	output := RedactSecrets(input)

	if strings.Contains(output, "abcd1234") {
		t.Fatal("token value was not redacted")
	}
	if strings.Contains(output, "abc.def.ghi") {
		t.Fatal("bearer token was not redacted")
	}
	if strings.Contains(output, "ghp_1234567890123456789012") {
		t.Fatal("github token was not redacted")
	}
	if strings.Contains(output, "github_pat_abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatal("fine-grained github token was not redacted")
	}
}
