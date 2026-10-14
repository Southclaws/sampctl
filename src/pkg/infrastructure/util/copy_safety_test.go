package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileKeepsSourceIndependent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source.pwn")
	target := filepath.Join(dir, "tmpl.pwn")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(source, target); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(target, []byte("changed target"), 0o600); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(source)
	if err != nil || string(contents) != "original" {
		t.Fatalf("source after editing copy = %q, %v; want original", contents, err)
	}
}

func TestCopyFileReplacesHardlinkedDestination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "first.pwn")
	second := filepath.Join(dir, "second.pwn")
	target := filepath.Join(dir, "tmpl.pwn")
	for path, contents := range map[string]string{first: "first source", second: "second source"} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(first, target); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	if err := CopyFile(second, target); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{first: "first source", second: "second source", target: "second source"} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Errorf("read %s = %q, %v; want %q", filepath.Base(path), contents, err, want)
		}
	}
}
