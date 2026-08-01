package development

import (
	"errors"
	"strings"
	"testing"
)

const validPatch = "diff --git a/internal/order.go b/internal/order.go\nindex 1111111..2222222 100644\n--- a/internal/order.go\n+++ b/internal/order.go\n@@ -1 +1 @@\n-old\n+new\n"

func TestValidateProposalPatch(t *testing.T) {
	patch, files, err := validateProposalPatch(validPatch, []string{"internal"})
	if err != nil {
		t.Fatalf("validate patch: %v", err)
	}
	if patch != validPatch || len(files) != 1 || files[0].Path != "internal/order.go" || files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Fatalf("unexpected patch result: %#v", files)
	}
}

func TestValidateProposalPatchRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name    string
		patch   string
		allowed []string
		want    error
	}{
		{name: "outside scope", patch: validPatch, allowed: []string{"cmd"}, want: ErrPathNotAllowed},
		{name: "path traversal", patch: strings.ReplaceAll(validPatch, "internal/order.go", "../order.go"), allowed: []string{"internal"}, want: ErrInvalidPatch},
		{name: "sensitive path", patch: strings.ReplaceAll(validPatch, "internal/order.go", ".env"), allowed: []string{".env"}, want: ErrSensitiveContent},
		{name: "secret addition", patch: strings.Replace(validPatch, "+new", "+api_key=abcdefghijk", 1), allowed: []string{"internal"}, want: ErrSensitiveContent},
		{name: "rename metadata", patch: strings.Replace(validPatch, "index 1111111..2222222 100644", "similarity index 100%", 1), allowed: []string{"internal"}, want: ErrInvalidPatch},
		{name: "executable mode", patch: strings.Replace(validPatch, "100644", "100755", 1), allowed: []string{"internal"}, want: ErrInvalidPatch},
		{name: "missing final newline", patch: strings.TrimSuffix(validPatch, "\n"), allowed: []string{"internal"}, want: ErrInvalidPatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := validateProposalPatch(test.patch, test.allowed)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidateProposalPatchRejectsIncorrectHunkCounts(t *testing.T) {
	patch := strings.Replace(validPatch, "@@ -1 +1 @@", "@@ -1,2 +1 @@", 1)
	if _, _, err := validateProposalPatch(patch, []string{"internal"}); !errors.Is(err, ErrInvalidPatch) {
		t.Fatalf("got %v, want invalid patch", err)
	}
}
