package runtime

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginArchiveMatchesSkipDirectoryHeaders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	filename := filepath.Join(t.TempDir(), "resource.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"includes/", "includes/first.inc", "includes/second.inc"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name != "includes/" {
			if _, err := entry.Write([]byte(name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	request := EnsureVersionedPluginRequest{Context: context.Background(), Dir: root}
	if _, err := extractPluginArchive(request, filename, ".zip", map[string]string{`^includes/`: ""}); err != nil {
		t.Fatal(err)
	}
	checkRuntimeTestFile(t, filepath.Join(root, "first.inc"), "includes/first.inc")
	checkRuntimeTestFile(t, filepath.Join(root, "second.inc"), "includes/second.inc")
}

func TestPluginArchiveInstallsEveryMatchingFile(t *testing.T) {
	t.Parallel()
	for _, archive := range []struct {
		name   string
		ext    string
		create func(*testing.T, string, map[string]string)
	}{
		{name: "zip", ext: ".zip", create: createRuntimeZipArchive},
		{name: "tar", ext: ".gz", create: createRuntimeTgzArchive},
	} {
		t.Run(archive.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			filename := filepath.Join(t.TempDir(), "resource"+archive.ext)
			archive.create(t, filename, map[string]string{
				"includes/first.inc":  "first include",
				"includes/second.inc": "second include",
				"plugins/first.so":    "first plugin",
				"plugins/second.so":   "second plugin",
				"misc/first.cfg":      "first configuration",
				"misc/second.cfg":     "second configuration",
			})
			paths := map[string]string{
				`^includes/.*\.inc$`: "",
				`^plugins/.*\.so$`:   "plugins/",
				`^misc/.*\.cfg$`:     "config/default.cfg",
			}
			request := EnsureVersionedPluginRequest{Context: context.Background(), Dir: root}
			if _, err := extractPluginArchive(request, filename, archive.ext, paths); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]string{
				"first.inc":          "first include",
				"second.inc":         "second include",
				"plugins/first.so":   "first plugin",
				"plugins/second.so":  "second plugin",
				"config/default.cfg": "second configuration",
			} {
				checkRuntimeTestFile(t, filepath.Join(root, path), want)
			}
			writeRuntimeTestFile(t, filepath.Join(root, "first.inc"), "user include")
			writeRuntimeTestFile(t, filepath.Join(root, "plugins/first.so"), "user plugin")
			request.IgnorePatterns = []string{"first.inc", "first.so"}
			if _, err := extractPluginArchive(request, filename, archive.ext, paths); err != nil {
				t.Fatal(err)
			}
			checkRuntimeTestFile(t, filepath.Join(root, "first.inc"), "user include")
			checkRuntimeTestFile(t, filepath.Join(root, "plugins/first.so"), "user plugin")
			request.IgnorePatterns = []string{"second.inc", "second.so"}
			result, err := extractPluginArchive(request, filename, archive.ext, paths)
			if err != nil {
				t.Fatal(err)
			}
			for source, target := range map[string]string{
				`^includes/.*\.inc$`: "first.inc",
				`^plugins/.*\.so$`:   "plugins/first.so",
			} {
				if got := result[source]; got != filepath.Join(root, target) {
					t.Errorf("extracted %s = %q, want %q", source, got, filepath.Join(root, target))
				}
			}
		})
	}
}

func TestPluginArchiveValidatesEveryMatchingDestination(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	filename := filepath.Join(t.TempDir(), "resource.zip")
	createRuntimeZipArchive(t, filename, map[string]string{
		"first.so":  "replacement first",
		"second.so": "replacement second",
	})
	external := filepath.Join(t.TempDir(), "first.so")
	writeRuntimeTestFile(t, external, "external first")
	writeRuntimeTestFile(t, filepath.Join(root, "plugins/second.so"), "previous second")
	if err := os.Symlink(external, filepath.Join(root, "plugins/first.so")); err != nil {
		t.Skipf("creating symlink: %v", err)
	}
	request := EnsureVersionedPluginRequest{Context: context.Background(), Dir: root}
	if _, err := extractPluginArchive(request, filename, ".zip", map[string]string{`.*\.so$`: "plugins/"}); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("extractPluginArchive(symlink destination) = %v, want os.ErrInvalid", err)
	}
	checkRuntimeTestFile(t, external, "external first")
	checkRuntimeTestFile(t, filepath.Join(root, "plugins/second.so"), "previous second")
}
