package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateOldConfig_CopiesAndRemovesOldDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	oldDir := filepath.Join(tmpHome, ".samp")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old dir: %v", err)
	}
	oldFile := filepath.Join(oldDir, "config.json")
	if err := os.WriteFile(oldFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	newDir := t.TempDir()
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}

	if err := MigrateOldConfig(newDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(newDir, "config.json")); err != nil {
		t.Fatalf("expected file in new dir: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old dir removed, stat err=%v", err)
	}
}

func TestMigrateOldConfig_NoOldDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	newDir := t.TempDir()
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}

	if err := MigrateOldConfig(newDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}
