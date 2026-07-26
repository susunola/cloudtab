package main

import "testing"

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
