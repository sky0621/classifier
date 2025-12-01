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
	alphaImg := strings.Repeat("a", 2*1024*1024)
	bravoImg := strings.Repeat("b", 2*1024*1024)
	writeFile(t, src, "alpha.jpg", alphaImg)
	writeFile(t, src, "bravo.png", bravoImg)
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

	assertFileContent(t, filepath.Join(dest, "images", "alpha.jpg"), alphaImg)
	assertFileContent(t, filepath.Join(dest, "images", "bravo.png"), bravoImg)
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
	rootContent := strings.Repeat("r", 2*1024*1024)
	nestedContent := strings.Repeat("n", 2*1024*1024)
	writeFile(t, src, "alpha.jpg", rootContent)

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "alpha.jpg", nestedContent)

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

	if root != rootContent || dupe != nestedContent {
		t.Fatalf("unexpected duplicate handling, got alpha.jpg=%q alpha_1.jpg=%q", root, dupe)
	}
}

func TestCLI_SkipsDuplicateImagesByContent(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	duplicate := strings.Repeat("d", 2*1024*1024)
	writeFile(t, src, "alpha.jpg", duplicate)

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "beta.jpg", duplicate)

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

	assertFileContent(t, filepath.Join(imagesDir, "alpha.jpg"), duplicate)

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

func TestCLI_DefaultConfigUsedWhenNoFlag(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	imgContent := strings.Repeat("i", 2*1024*1024)
	writeFile(t, src, "alpha.jpg", imgContent)
	writeFile(t, src, "bravo.txt", "doc")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	assertFileContent(t, filepath.Join(dest, "images", "alpha.jpg"), imgContent)
	assertFileContent(t, filepath.Join(dest, "documents", "bravo.txt"), "doc")

	if _, err := os.Stat(filepath.Join(dest, "warn.csv")); err == nil {
		t.Fatalf("warn.csv should not exist when nothing skipped")
	}
}

func TestCLI_MultipleDuplicateImagesProduceMultipleWarnings(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	duplicate := strings.Repeat("x", 2*1024*1024)
	writeFile(t, src, "alpha.jpg", duplicate)
	writeFile(t, src, "bravo.jpg", duplicate)

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "charlie.jpg", duplicate)

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	warnPath := filepath.Join(dest, "warn.csv")
	content := readFile(t, warnPath)
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 warning lines, got %d", len(lines))
	}

	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			t.Fatalf("expected 2 columns in warn.csv, got %d", len(parts))
		}
		if parts[1] != filepath.Join(dest, "images", "alpha.jpg") {
			t.Fatalf("expected dest to reference first copied image, got %s", parts[1])
		}
	}
}

func TestCLI_SkipsDuplicateMoviesByContent(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	content := strings.Repeat("v", 2*1024*1024)
	writeFile(t, src, "alpha.mp4", content)

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "beta.mp4", content)

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	moviesDir := filepath.Join(dest, "movies")
	entries, err := os.ReadDir(moviesDir)
	if err != nil {
		t.Fatalf("expected movies directory, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in movies, got %d", len(entries))
	}

	assertFileContent(t, filepath.Join(moviesDir, "alpha.mp4"), content)

	warnPath := filepath.Join(dest, "warn.csv")
	warnContent := readFile(t, warnPath)
	lines := strings.Split(strings.TrimSpace(warnContent), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 warning line, got %d", len(lines))
	}
	parts := strings.Split(lines[0], ",")
	if len(parts) != 2 {
		t.Fatalf("expected 2 columns in warn.csv, got %d", len(parts))
	}
	if parts[0] != filepath.Join(src, "nested", "beta.mp4") {
		t.Fatalf("unexpected src in warn.csv: %s", parts[0])
	}
	if parts[1] != filepath.Join(dest, "movies", "alpha.mp4") {
		t.Fatalf("unexpected dest in warn.csv: %s", parts[1])
	}
}

func TestCLI_SkipsDuplicateDocumentsByContent(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	writeFile(t, src, "report.txt", "doc")

	nested := filepath.Join(src, "nested")
	mustMkdir(t, nested)
	writeFile(t, nested, "copy.log", "doc")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	docsDir := filepath.Join(dest, "documents")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("expected documents directory, got: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in documents, got %d", len(entries))
	}

	keptName := entries[0].Name()
	keptPath := filepath.Join(docsDir, keptName)
	assertFileContent(t, keptPath, "doc")

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
	skippedSrc := parts[0]
	existingDest := parts[1]

	expectedKept := filepath.Join(dest, "documents", entries[0].Name())
	if existingDest != expectedKept {
		t.Fatalf("unexpected dest in warn.csv: %s", existingDest)
	}

	expectedSkipped := filepath.Join(src, "nested", "copy.log")
	alternativeSkipped := filepath.Join(src, "report.txt")
	if skippedSrc != expectedSkipped && skippedSrc != alternativeSkipped {
		t.Fatalf("unexpected src in warn.csv: %s", skippedSrc)
	}
}

func TestCLI_SkipsSmallImagesBySize(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	// 512 KB - should be skipped
	small := strings.Repeat("a", 512*1024)
	writeFile(t, src, "small.jpg", small)
	// 2 MB - should be copied
	large := strings.Repeat("b", 2*1024*1024)
	writeFile(t, src, "large.jpg", large)

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	imagesDir := filepath.Join(dest, "images")
	if _, err := os.Stat(filepath.Join(imagesDir, "small.jpg")); err == nil {
		t.Fatalf("expected small image to be skipped")
	}
	assertFileContent(t, filepath.Join(imagesDir, "large.jpg"), large)

	if _, err := os.Stat(filepath.Join(dest, "warn.csv")); err == nil {
		t.Fatalf("warn.csv should not exist when only small images are skipped")
	}
}

func TestCLI_PlacesImagesAndMoviesInDateFolders(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	imgContent := strings.Repeat("p", 2*1024*1024)
	writeFile(t, src, "2024-01-31_photo.jpg", imgContent)
	writeFile(t, src, "IMG_20230715_video.mp4", "video")

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	assertFileContent(t, filepath.Join(dest, "images", "2024", "202401", "2024-01-31_photo.jpg"), imgContent)
	assertFileContent(t, filepath.Join(dest, "movies", "2023", "202307", "IMG_20230715_video.mp4"), "video")
}

func TestCLI_CopiesFilesWhenNoDateFound(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	dest := filepath.Join(workspace, "dest")

	mustMkdir(t, src)
	nested := filepath.Join(src, "misc")
	mustMkdir(t, nested)

	imgContent := strings.Repeat("y", 2*1024*1024)
	writeFile(t, nested, "picture.jpg", imgContent)

	res := runCLI(t, workspace, absPath(t, src), absPath(t, dest))
	if res.err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", res.err, res.stderr)
	}

	// No date in dir or filename; file should be placed directly under images.
	assertFileContent(t, filepath.Join(dest, "images", "picture.jpg"), imgContent)
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
