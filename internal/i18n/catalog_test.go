package i18n

import "testing"

// TestCatalogsParity ensures the en stub mirrors the de keyset exactly, so no
// key silently falls through to the de fallback unnoticed (and vice-versa).
func TestCatalogsParity(t *testing.T) {
	de := catalogs[DE]
	en := catalogs[EN]
	for k := range de.strings {
		if _, ok := en.strings[k]; !ok {
			t.Errorf("en missing string key %q present in de", k)
		}
	}
	for k := range en.strings {
		if _, ok := de.strings[k]; !ok {
			t.Errorf("de missing string key %q present in en", k)
		}
	}
	for k := range de.plurals {
		if _, ok := en.plurals[k]; !ok {
			t.Errorf("en missing plural key %q present in de", k)
		}
	}
	for k := range en.plurals {
		if _, ok := de.plurals[k]; !ok {
			t.Errorf("de missing plural key %q present in en", k)
		}
	}
}
