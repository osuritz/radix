package radix_test

import (
	"io/fs"
	"testing"

	radix "github.com/osuritz/radix"
)

// TestUIAssetsGitkeepPresent verifies that ui/dist/.gitkeep exists in the
// embedded UIAssets FS. This placeholder must never be accidentally removed: its
// absence causes the go:embed directive to fail silently (empty FS) or with a
// compile error, producing a confusing build failure rather than an obvious test
// failure. If this test fails, restore ui/dist/.gitkeep and re-run make build.
func TestUIAssetsGitkeepPresent(t *testing.T) {
	const target = "ui/dist/.gitkeep"

	if _, err := fs.Stat(radix.UIAssets, target); err != nil {
		t.Fatalf("UIAssets is missing %q — restore the placeholder file so the embed directive remains valid: %v", target, err)
	}
}
