package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_CopiesDirectFiles(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "alpha.txt", "alpha")
	writeFile(t, src, "bravo.log", "bravo")

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "charlie.txt", "charlie")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("expected destination directory, got: %v", err)
	}

	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		names[entry.Name()] = true
	}

	if !names["alpha.txt"] || !names["bravo.log"] {
		t.Fatalf("expected alpha.txt and bravo.log to be copied, got: %v", names)
	}
	if names["nested"] {
		t.Fatalf("did not expect nested directory to be copied")
	}
	if _, err := os.Stat(filepath.Join(dest, "nested", "charlie.txt")); err == nil {
		t.Fatalf("did not expect nested files to be copied")
	}

	if got := readFile(t, filepath.Join(dest, "alpha.txt")); got != "alpha" {
		t.Fatalf("unexpected content for alpha.txt: %s", got)
	}
	if got := readFile(t, filepath.Join(dest, "bravo.log")); got != "bravo" {
		t.Fatalf("unexpected content for bravo.log: %s", got)
	}
}

func TestCLI_RejectsRelativePaths(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "alpha.txt", "alpha")

	res := runCLI(t, workspace, "src", "dest")
	if res.exitCode == 0 {
		t.Fatalf("expected non-zero exit for relative paths")
	}
	if !strings.Contains(res.stderr, "absolute") {
		t.Fatalf("expected message to mention absolute paths, stderr: %s", res.stderr)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatalf("destination should not be created when validation fails")
	}
}

type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
	err      error
}

func runCLI(t *testing.T, workdir string, args ...string) cliResult {
	t.Helper()

	cmd := exec.Command("go", "run", filepath.Join(repoRoot(t), "cmd", "classifier"))
	cmd.Args = append(cmd.Args, args...)
	cmd.Dir = repoRoot(t)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return cliResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		err:      err,
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("could not find go.mod from %s", dir)
		}
		dir = next
	}
}

func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to get absolute path for %s: %v", path, err)
	}
	return abs
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", fullPath, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(content)
}
