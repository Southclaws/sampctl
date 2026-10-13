package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type atomicFailureReader struct{ err error }

func (reader atomicFailureReader) Read([]byte) (int, error) { return 0, reader.err }

func TestWriteFromReaderAtomicRetainsPreviousFileOnReadFailure(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "installed")
	if err := os.WriteFile(path, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("read interrupted")
	reader := io.MultiReader(strings.NewReader("partial replacement"), atomicFailureReader{err: failure})
	if err := WriteFromReaderAtomic(path, reader, PermDirPrivate, PermFileShared); !errors.Is(err, failure) {
		t.Errorf("WriteFromReaderAtomic(read failure) = %v, want underlying read error", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "previous" {
		t.Errorf("installed file = %q, want previous", contents)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "installed" {
		t.Errorf("files after failed write = %v, want only installed", entries)
	}
}
