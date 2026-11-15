package migrations

import "testing"

func TestLatestVersionFromEmbeddedMigrations(t *testing.T) {
	if got := LatestVersion(); got < 1 {
		t.Fatalf("LatestVersion()=%d", got)
	}
}
