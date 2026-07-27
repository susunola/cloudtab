package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSiteFlag(t *testing.T) {
	// Accepted values (case-insensitive, surrounding whitespace tolerated).
	valid := []string{
		"", "domestic", "Domestic", "DOMESTIC", "cn", "china",
		"intl", "INTL", " Intl ", "international", "global", "overseas",
	}
	for _, v := range valid {
		if err := validateSiteFlag("--aws-site", v); err != nil {
			t.Errorf("validateSiteFlag(%q) = %v, want nil", v, err)
		}
	}

	// Rejected values: genuine typos / unsupported strings. These previously
	// silently fell through to the cn partition and only failed at auth time.
	invalid := []string{
		"foo", "domesitc", "chiana", "internal", "globl", "itl", "cn-hangzhou",
	}
	for _, v := range invalid {
		if err := validateSiteFlag("--aws-site", v); err == nil {
			t.Errorf("validateSiteFlag(%q) = nil, want error", v)
		}
	}
}

// TestClearCache verifies the `cache clear` action removes an existing cache
// file and is a no-op (no error) when the file is absent.
func TestClearCache(t *testing.T) {
	// Missing file -> success, nothing to remove.
	if err := clearCache(filepath.Join(t.TempDir(), "does-not-exist.db")); err != nil {
		t.Fatalf("clearCache on missing file = %v, want nil", err)
	}

	// Existing file -> removed.
	dir := t.TempDir()
	p := filepath.Join(dir, "cache.db")
	if err := os.WriteFile(p, []byte("bolt-data"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if err := clearCache(p); err != nil {
		t.Fatalf("clearCache = %v, want nil", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("cache file should be gone after clear (stat err = %v)", err)
	}

	// Empty path -> error.
	if err := clearCache(""); err == nil {
		t.Error("clearCache(\"\") = nil, want error")
	}
}
