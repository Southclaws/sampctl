package download

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMigrateOldConfig_CopiesAndRemovesOldDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	if home, err := migrationHomeDir(); err != nil || home != tmpHome {
		t.Fatalf("migrationHomeDir() = %q, %v, want temporary home %q, nil", home, err, tmpHome)
	}

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
	if home, err := migrationHomeDir(); err != nil || home != tmpHome {
		t.Fatalf("migrationHomeDir() = %q, %v, want temporary home %q, nil", home, err, tmpHome)
	}

	newDir := t.TempDir()
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir new dir: %v", err)
	}

	if err := MigrateOldConfig(newDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestMigrationHomeDirUsesCurrentEnvironment(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	t.Setenv("HOME", first)
	t.Setenv("USERPROFILE", second)
	home, err := migrationHomeDir()
	if err != nil || home != first {
		t.Fatalf("migrationHomeDir() = %q, %v, want HOME %q, nil", home, err, first)
	}

	t.Setenv("HOME", second)
	home, err = migrationHomeDir()
	if err != nil || home != second {
		t.Fatalf("migrationHomeDir() = %q, %v, want updated HOME %q, nil", home, err, second)
	}
}

func TestMigrationHomeDirFallsBackToCurrentUser(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Skipf("current user lookup unavailable: %v", err)
	}

	if current.HomeDir == "" {
		t.Skip("current user has no home directory")
	}

	for _, key := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "home"} {
		t.Setenv(key, "")
	}

	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("native home lookup has a platform default")
	}

	home, err := migrationHomeDir()
	if err != nil || home != current.HomeDir {
		t.Fatalf("migrationHomeDir() = %q, %v, want account home %q, nil", home, err, current.HomeDir)
	}
}
