//go:build darwin
// +build darwin

package subpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinVolumeCleanupPreservesSharedDirectory(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.Mkdir(shared, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(shared, "retained")
	if err := os.WriteFile(file, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	sp := New(nil)
	if err := sp.CleanSubPaths(root, "shared"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "retained" {
		t.Fatal("cleanup changed the shared data", err)
	}
	if _, _, err := sp.PrepareSafeSubpath(Subpath{Path: shared, VolumePath: root}); err == nil {
		t.Fatal("unsupported subpath mount was accepted")
	}
	if err := sp.SafeMakeDir(filepath.Join(shared, "child"), root, 0700); err == nil {
		t.Fatal("unsupported subpath directory creation was accepted")
	}
}
