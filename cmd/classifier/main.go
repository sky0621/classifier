package main

import (
	"crypto/sha256"
	"embed"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	Categories      []category `yaml:"categories"`
	DefaultCategory string     `yaml:"default_category"`
	DatePatterns    []string   `yaml:"date_patterns"`
}

type category struct {
	Name       string   `yaml:"name"`
	Extensions []string `yaml:"extensions"`
}

const minImageSize int64 = 1 << 20 // 1 MiB

//go:embed config.yaml
var embeddedFS embed.FS

type skippedEntry struct {
	srcPath  string
	destPath string
}

type dateResolver struct {
	patterns []*regexp.Regexp
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flagSet := flag.NewFlagSet("classifier", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	var configPath string
	flagSet.StringVar(&configPath, "config", "", "path to YAML config file")
	flagSet.StringVar(&configPath, "c", "", "path to YAML config file")

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return err
	}

	if flagSet.NArg() != 2 {
		return usageError("expected 2 arguments: <src-abs-dir> <dest-abs-dir>")
	}

	src := flagSet.Arg(0)
	dest := flagSet.Arg(1)

	if !filepath.IsAbs(src) || !filepath.IsAbs(dest) {
		return usageError("source and destination must be absolute paths")
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	resolver := newCategoryResolver(cfg)
	dateResolver, err := newDateResolver(cfg.DatePatterns)
	if err != nil {
		return err
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

	hashIndex := make(map[string]string)
	var skipped []skippedEntry

	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat source entry %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			// Skip non-regular files (symlinks, devices, etc.).
			return nil
		}

		name := d.Name()
		category := resolver.categoryFor(name)

		if category == "images" && info.Size() < minImageSize {
			// Skip tiny images to avoid noise.
			return nil
		}

		hash, err := fileHash(path)
		if err != nil {
			return err
		}
		if existingPath, exists := hashIndex[hash]; exists {
			skipped = append(skipped, skippedEntry{srcPath: path, destPath: existingPath})
			return nil
		}

		targetDir := filepath.Join(dest, category)
		if category == "images" || category == "movies" {
			if year, ym, ok := dateResolver.resolve(name); ok {
				targetDir = filepath.Join(targetDir, year, ym)
			}
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("create category directory %s: %w", targetDir, err)
		}

		finalPath, err := uniqueDestPath(targetDir, name)
		if err != nil {
			return err
		}

		if err := copyFile(path, finalPath, info.Mode()); err != nil {
			return err
		}

		hashIndex[hash] = finalPath

		return nil
	})

	if walkErr != nil {
		return walkErr
	}

	if len(skipped) > 0 {
		if err := writeWarnings(filepath.Join(dest, "warn.csv"), skipped); err != nil {
			return err
		}
	}

	return nil
}

func usageError(msg string) error {
	return errors.New(msg + "; usage: classifier [-config path|-c path] <src-abs-dir> <dest-abs-dir>")
}

func loadConfig(path string) (config, error) {
	if path == "" {
		return loadEmbeddedConfig()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func loadEmbeddedConfig() (config, error) {
	var cfg config
	data, err := embeddedFS.ReadFile("config.yaml")
	if err != nil {
		return config{}, fmt.Errorf("read embedded config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("parse embedded config: %w", err)
	}

	return cfg, nil
}

type categoryResolver struct {
	defaultCategory string
	extToCategory   map[string]string
}

func newCategoryResolver(cfg config) categoryResolver {
	resolver := categoryResolver{
		defaultCategory: cfg.DefaultCategory,
		extToCategory:   map[string]string{},
	}
	if resolver.defaultCategory == "" {
		resolver.defaultCategory = "others"
	}

	for _, cat := range cfg.Categories {
		for _, ext := range cat.Extensions {
			clean := strings.TrimPrefix(strings.ToLower(ext), ".")
			if clean == "" {
				continue
			}
			resolver.extToCategory[clean] = cat.Name
		}
	}

	return resolver
}

func (r categoryResolver) categoryFor(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if ext == "" {
		return r.defaultCategory
	}
	if cat, ok := r.extToCategory[ext]; ok {
		return cat
	}
	return r.defaultCategory
}

func newDateResolver(patterns []string) (dateResolver, error) {
	if len(patterns) == 0 {
		return dateResolver{}, nil
	}

	res := dateResolver{patterns: make([]*regexp.Regexp, 0, len(patterns))}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return dateResolver{}, fmt.Errorf("compile date pattern %q: %w", p, err)
		}
		res.patterns = append(res.patterns, re)
	}
	return res, nil
}

func (r dateResolver) resolve(name string) (string, string, bool) {
	for _, re := range r.patterns {
		matches := re.FindStringSubmatch(name)
		if matches == nil {
			continue
		}

		subexpNames := re.SubexpNames()
		var year, month string
		for i, v := range subexpNames {
			if v == "year" {
				year = matches[i]
			}
			if v == "month" {
				month = matches[i]
			}
		}
		if year == "" && len(matches) >= 3 {
			year = matches[1]
			month = matches[2]
		}
		if year == "" || month == "" {
			year, month = fallbackYearMonth(matches)
		}
		if len(year) == 4 && len(month) == 2 {
			return year, year + month, true
		}
	}
	return "", "", false
}

func fallbackYearMonth(matches []string) (string, string) {
	var year, month string
	for _, m := range matches {
		if len(m) == 4 && allDigits(m) && year == "" {
			year = m
			continue
		}
		if len(m) == 2 && allDigits(m) && month == "" {
			month = m
		}
		if year != "" && month != "" {
			break
		}
	}
	return year, month
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
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

func uniqueDestPath(dir, name string) (string, error) {
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return target, nil
		}
		return "", fmt.Errorf("stat destination %s: %w", target, err)
	}

	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)

	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return candidate, nil
			}
			return "", fmt.Errorf("stat destination %s: %w", candidate, err)
		}
	}
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for hash %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func writeWarnings(path string, entries []skippedEntry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write warnings: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	for _, e := range entries {
		if err := w.Write([]string{e.srcPath, e.destPath}); err != nil {
			return fmt.Errorf("write warnings: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write warnings: %w", err)
	}
	return nil
}
