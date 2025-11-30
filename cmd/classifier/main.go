package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return usageError("expected 2 arguments: <src-abs-dir> <dest-abs-dir>")
	}

	src := os.Args[1]
	dest := os.Args[2]

	if !filepath.IsAbs(src) || !filepath.IsAbs(dest) {
		return usageError("source and destination must be absolute paths")
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source entries: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Skip nested directories; only copy direct files.
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat source entry %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			// Skip non-regular files (symlinks, devices, etc.).
			continue
		}

		category := extCategory(entry.Name())
		targetDir := filepath.Join(dest, category)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("create category directory %s: %w", targetDir, err)
		}

		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(targetDir, entry.Name()), info.Mode()); err != nil {
			return err
		}
	}

	return nil
}

func usageError(msg string) error {
	return errors.New(msg + "; usage: classifier <src-abs-dir> <dest-abs-dir>")
}

func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("create destination file %s: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dest, err)
	}

	return nil
}

func extCategory(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return "no_ext"
	}
	return ext[1:]
}
