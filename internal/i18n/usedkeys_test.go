package i18n_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

// keyUse matches the literal keys the templates and view models look up:
// components.T(ctx, "x.y"), components.Tn(ctx, "x.y", n) and the bare
// i18n.T/Tn forms.
//
// The trailing [,)] matters: some call sites BUILD a key
// (T(ctx, "date.month."+strconv.Itoa(m))), and a prefix like "date.month."
// is not a key anyone could look up. Requiring the literal to be the whole
// argument keeps those out.
var keyUse = regexp.MustCompile(`(?:components\.|i18n\.)?T[n]?\(ctx, "([a-z0-9][a-zA-Z0-9._]*)"\s*[,)]`)

// TestEveryUsedKeyExists walks the WebUI sources and fails on a key that no
// catalog carries. The parity test only compares DE against EN — it cannot
// see a key that is missing from BOTH, which is exactly what happens when a
// surface is ported from another branch and its catalog entries are left
// behind. Two such keys shipped before this guard existed
// (cockpit.rail.facts, wissen.daily); they rendered as their own names on
// screen, which no test noticed and no compiler could.
func TestEveryUsedKeyExists(t *testing.T) {
	root := filepath.Join("..", "adapter", "webui")
	missing := map[string][]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// _templ.go is generated FROM .templ — scanning both would only
		// report the same key twice.
		if !strings.HasSuffix(path, ".templ") && !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, m := range keyUse.FindAllSubmatch(src, -1) {
			key := string(m[1])
			if !i18n.HasKey(key) {
				missing[key] = append(missing[key], path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) == 0 {
		return
	}
	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Errorf("i18n key %q is used in %v but exists in no catalog — it would render as its own name", k, missing[k])
	}
}
