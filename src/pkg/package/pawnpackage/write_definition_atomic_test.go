package pawnpackage

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteDefinitionMatchesEncodedDefinitionAndPreservesPermissions(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			pkg := Package{Format: format, LocalPath: t.TempDir(), User: "fixture", Repo: "project", Entry: "main.pwn"}
			path := filepath.Join(pkg.LocalPath, "pawn."+format)
			if err := os.WriteFile(path, []byte("previous definition"), 0o640); err != nil {
				t.Fatal(err)
			}
			encoded, err := pkg.EncodeDefinition()
			if err != nil {
				t.Fatal(err)
			}
			if err := pkg.WriteDefinition(); err != nil {
				t.Fatal(err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(contents, encoded) {
				t.Errorf("WriteDefinition(%s) = %s, want EncodeDefinition() = %s", format, contents, encoded)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
				t.Errorf("WriteDefinition(%s) permissions = %o, want 640", format, info.Mode().Perm())
			}
		})
	}
}

func TestWriteDefinitionRejectsSymlink(t *testing.T) {
	t.Parallel()
	pkg := Package{Format: "json", LocalPath: t.TempDir()}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(pkg.LocalPath, "pawn.json")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := pkg.WriteDefinition(); err == nil {
		t.Fatal("WriteDefinition through a symlink succeeded")
	}
	contents, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "keep" {
		t.Errorf("external definition = %q, want keep", contents)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("rejected definition write replaced the symlink")
	}
}

func TestWriteDefinitionRejectsDirectory(t *testing.T) {
	t.Parallel()
	pkg := Package{Format: "json", LocalPath: t.TempDir()}
	path := filepath.Join(pkg.LocalPath, "pawn.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pkg.WriteDefinition(); err == nil {
		t.Fatal("WriteDefinition over a directory succeeded")
	}
	if _, err := os.Stat(filepath.Join(path, "keep")); err != nil {
		t.Errorf("rejected definition write lost directory contents: %v", err)
	}
}
