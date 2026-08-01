package development

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestExternalSecretScannerEvidenceAndFailClosed(t *testing.T) {
	repository := t.TempDir()
	scanner, err := NewExternalSecretScanner(
		"test-enterprise-scanner", os.Args[0],
		[]string{"-test.run=TestSecretScannerHelperProcess", "--", "{repository}"},
		[]string{"EKBDA_TEST_SCANNER_RESULT"}, 5*time.Second,
	)
	if err != nil {
		t.Fatalf("create scanner: %v", err)
	}
	t.Setenv("EKBDA_TEST_SCANNER_RESULT", "pass")
	evidence, err := scanner.Scan(context.Background(), repository)
	if err != nil || !evidence.Passed || evidence.Scanner != "test-enterprise-scanner" || evidence.OutputSHA256 == "" {
		t.Fatalf("passing scan evidence: %#v, %v", evidence, err)
	}
	t.Setenv("EKBDA_TEST_SCANNER_RESULT", "reject")
	evidence, err = scanner.Scan(context.Background(), repository)
	if !errors.Is(err, ErrEnterpriseSecretScan) || evidence.Passed || evidence.OutputSHA256 == "" {
		t.Fatalf("rejected scan must fail closed: %#v, %v", evidence, err)
	}
}

func TestSecretScannerHelperProcess(t *testing.T) {
	result := os.Getenv("EKBDA_TEST_SCANNER_RESULT")
	if result == "" {
		return
	}
	fmt.Fprintln(os.Stdout, "redacted scanner evidence")
	if result != "pass" {
		os.Exit(3)
	}
}
