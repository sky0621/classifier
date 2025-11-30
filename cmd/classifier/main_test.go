package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_ClassifiesFilesByConfig(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "alpha.jpg", "alpha")
	writeFile(t, src, "bravo.png", "bravo")
	writeFile(t, src, "charlie.mp4", "charlie")
	writeFile(t, src, "delta.txt", "delta")
	writeFile(t, src, "echo", "noext")

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "foxtrot.txt", "foxtrot")

	configPath := filepath.Join(workspace, "config.yaml")
	writeFile(t, workspace, "config.yaml", `categories:
  - name: images
    extensions: [jpg, jpeg, png]
  - name: movies
    extensions:
      - mp4
      - mpeg
  - name: documents
    extensions: [txt, log]
default_category: others
`)

	res := runCLI(t, workspace, "-c", absPath(t, configPath), absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	assertFileContent(t, filepath.Join(dest, "images", "alpha.jpg"), "alpha")
	assertFileContent(t, filepath.Join(dest, "images", "bravo.png"), "bravo")
	assertFileContent(t, filepath.Join(dest, "movies", "charlie.mp4"), "charlie")
	assertFileContent(t, filepath.Join(dest, "documents", "delta.txt"), "delta")
	assertFileContent(t, filepath.Join(dest, "documents", "foxtrot.txt"), "foxtrot")
	assertFileContent(t, filepath.Join(dest, "others", "echo"), "noext")

	if _, err := os.Stat(filepath.Join(dest, "nested")); err == nil {
		t.Fatalf("did not expect nested directory structure to be preserved")
	}
}

func TestCLI_HandlesDuplicateFilenames(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "alpha.jpg", "root")

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "alpha.jpg", "nested")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	imagesDir := filepath.Join(dest, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		t.Fatalf("expected images directory, got: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files in images, got %d", len(entries))
	}

	root := readFile(t, filepath.Join(imagesDir, "alpha.jpg"))
	dupe := readFile(t, filepath.Join(imagesDir, "alpha_1.jpg"))

	if root != "root" || dupe != "nested" {
		t.Fatalf("unexpected duplicate handling, got alpha.jpg=%q alpha_1.jpg=%q", root, dupe)
	}
}

func TestCLI_SkipsDuplicateImagesByContent(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "alpha.jpg", "same")

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "beta.jpg", "same")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	imagesDir := filepath.Join(dest, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		t.Fatalf("expected images directory, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in images, got %d", len(entries))
	}

	assertFileContent(t, filepath.Join(imagesDir, "alpha.jpg"), "same")

	warnPath := filepath.Join(dest, "warn.csv")
	content := readFile(t, warnPath)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 warning line, got %d", len(lines))
	}
	parts := strings.Split(lines[0], ",")
	if len(parts) != 2 {
		t.Fatalf("expected 2 columns in warn.csv, got %d", len(parts))
	}
	srcPath := parts[0]
	destPath := parts[1]
	if srcPath != filepath.Join(src, "nested", "beta.jpg") {
		t.Fatalf("unexpected src in warn.csv: %s", srcPath)
	}
	if destPath != filepath.Join(dest, "images", "alpha.jpg") {
		t.Fatalf("unexpected dest in warn.csv: %s", destPath)
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

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got := readFile(t, path)
	if got != want {
		t.Fatalf("unexpected content for %s: got %q want %q", path, got, want)
	}
}
