package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFixWithCategoryFlag(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.svg")
	outputPath := filepath.Join(dir, "output.svg")
	input := []byte(`<svg width="100" height="50"><script>alert(1)</script><rect onclick="x()" /></svg>`)
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"fix", "--fix=metadata,safety", "--unsafe", "-o", outputPath, inputPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := string(out)
	for _, removed := range []string{"<script", "onclick"} {
		if strings.Contains(got, removed) {
			t.Fatalf("expected %q to be removed, got %s", removed, got)
		}
	}
	for _, added := range []string{`xmlns="http://www.w3.org/2000/svg"`, `viewBox="0 0 100 50"`} {
		if !strings.Contains(got, added) {
			t.Fatalf("expected %q in output, got %s", added, got)
		}
	}
}

func TestRunFixRejectsUnknownCategoryBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.svg")
	outputPath := filepath.Join(dir, "output.svg")
	if err := os.WriteFile(inputPath, []byte(`<svg width="100" height="50"></svg>`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"fix", "--fix=metadata,,safety", "-o", outputPath, inputPath}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("Run returned %d, want 2; stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("expected no output file, stat err=%v", err)
	}
	if !strings.Contains(stderr.String(), "empty fix category") {
		t.Fatalf("expected category error, got %s", stderr.String())
	}
}
