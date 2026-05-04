package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDownloadsDirSupportsRepoRootAndServerCwd(t *testing.T) {
	root := t.TempDir()
	releases := filepath.Join(root, "releases")
	if err := os.Mkdir(releases, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if got := resolveDownloadsDir(); filepath.Clean(got) != filepath.Clean("releases") {
		t.Fatalf("repo root cwd downloads dir = %q, want releases", got)
	}

	serverDir := filepath.Join(root, "server")
	if err := os.Mkdir(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(serverDir); err != nil {
		t.Fatal(err)
	}
	if got := resolveDownloadsDir(); filepath.Clean(got) != filepath.Clean("../releases") {
		t.Fatalf("server cwd downloads dir = %q, want ../releases", got)
	}
}
