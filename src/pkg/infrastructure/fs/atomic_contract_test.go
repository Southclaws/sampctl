package fs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteAtomicReplacesExistingFile(t *testing.T) {
	t.Parallel()

	for _, writer := range atomicContractWriters() {
		t.Run(writer.name, func(t *testing.T) {
			for _, payload := range []struct {
				name string
				data []byte
			}{
				{name: "nil"},
				{name: "empty", data: []byte{}},
				{name: "nonempty", data: []byte("replacement")},
			} {
				t.Run(payload.name, func(t *testing.T) {
					directory := t.TempDir()
					path := filepath.Join(directory, "installed")
					if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
						t.Fatal(err)
					}
					previousFile, err := os.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					previous, err := previousFile.Stat()
					closeErr := previousFile.Close()
					if err != nil {
						t.Fatal(err)
					}
					if closeErr != nil {
						t.Fatal(closeErr)
					}

					if err := writer.write(path, payload.data, PermDirPrivate, 0o640); err != nil {
						t.Fatal(err)
					}

					assertAtomicContractFile(t, path, payload.data)
					current, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					if os.SameFile(previous, current) {
						t.Error("replacement reused the previous file")
					}
					if runtime.GOOS != "windows" && current.Mode().Perm() != 0o640 {
						t.Errorf("replacement permissions = %o, want 640", current.Mode().Perm())
					}
					assertAtomicContractEntries(t, directory, "installed")
				})
			}
		})
	}
}

func TestWriteAtomicRenameFailurePreservesDestination(t *testing.T) {
	t.Parallel()

	for _, writer := range atomicContractWriters() {
		t.Run(writer.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "installed")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			child := filepath.Join(path, "keep")
			if err := os.WriteFile(child, []byte("previous"), 0o600); err != nil {
				t.Fatal(err)
			}

			err := writer.write(path, []byte("replacement"), PermDirPrivate, PermFileShared)
			var renameError *os.LinkError
			if !errors.As(err, &renameError) {
				t.Fatalf("write over directory = %v, want rename error", err)
			}
			if renameError.New != path {
				t.Errorf("rename destination = %q, want %q", renameError.New, path)
			}

			assertAtomicContractFile(t, child, []byte("previous"))
			assertAtomicContractEntries(t, path, "keep")
			assertAtomicContractEntries(t, directory, "installed")
		})
	}
}

func TestWriteAtomicReplacesLinkedDestination(t *testing.T) {
	t.Parallel()

	for _, writer := range atomicContractWriters() {
		t.Run(writer.name, func(t *testing.T) {
			for _, link := range []struct {
				name   string
				create func(string, string) error
			}{
				{name: "symlink", create: os.Symlink},
				{name: "hardlink", create: os.Link},
			} {
				t.Run(link.name, func(t *testing.T) {
					directory := t.TempDir()
					original := filepath.Join(directory, "original")
					path := filepath.Join(directory, "installed")
					if err := os.WriteFile(original, []byte("previous"), 0o600); err != nil {
						t.Fatal(err)
					}
					if err := link.create(original, path); err != nil {
						t.Skipf("%s unavailable: %v", link.name, err)
					}

					if err := writer.write(path, []byte("replacement"), PermDirPrivate, PermFileShared); err != nil {
						t.Fatal(err)
					}

					assertAtomicContractFile(t, original, []byte("previous"))
					assertAtomicContractFile(t, path, []byte("replacement"))
					originalInfo, err := os.Stat(original)
					if err != nil {
						t.Fatal(err)
					}
					current, err := os.Lstat(path)
					if err != nil {
						t.Fatal(err)
					}
					if !current.Mode().IsRegular() || os.SameFile(originalInfo, current) {
						t.Error("replacement is not an independent regular file")
					}
					if runtime.GOOS != "windows" && originalInfo.Mode().Perm() != 0o600 {
						t.Errorf("original permissions = %o, want 600", originalInfo.Mode().Perm())
					}
					assertAtomicContractEntries(t, directory, "installed", "original")
				})
			}
		})
	}
}

func TestWriteAtomicReaderOwnershipAndSetupOrder(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"success", "read failure", "invalid parent"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "installed")
			failure := errors.New("read interrupted")
			reader := &atomicContractReadCloser{reader: bytes.NewReader([]byte("replacement"))}
			switch name {
			case "read failure":
				reader.reader = atomicFailureReader{err: failure}
			case "invalid parent":
				parent := filepath.Join(directory, "parent")
				if err := os.WriteFile(parent, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(parent, "installed")
			}

			err := WriteFromReaderAtomic(path, reader, PermDirPrivate, PermFileShared)
			switch name {
			case "success":
				if err != nil {
					t.Fatal(err)
				}
				assertAtomicContractFile(t, path, []byte("replacement"))
			case "read failure":
				if !errors.Is(err, failure) {
					t.Errorf("write = %v, want read error", err)
				}
			case "invalid parent":
				if err == nil {
					t.Error("write through regular-file parent succeeded")
				}
				if reader.reads != 0 {
					t.Errorf("reads after setup failure = %d, want zero", reader.reads)
				}
				assertAtomicContractFile(t, filepath.Dir(path), []byte("keep"))
			}
			if reader.closes != 0 {
				t.Errorf("reader closes = %d, want zero", reader.closes)
			}
		})
	}
}

func atomicContractWriters() []struct {
	name  string
	write func(string, []byte, os.FileMode, os.FileMode) error
} {
	return []struct {
		name  string
		write func(string, []byte, os.FileMode, os.FileMode) error
	}{
		{name: "bytes", write: WriteFileAtomic},
		{name: "reader", write: func(path string, data []byte, dirPerm, filePerm os.FileMode) error {
			return WriteFromReaderAtomic(path, bytes.NewReader(data), dirPerm, filePerm)
		}},
	}
}

func assertAtomicContractFile(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("contents of %s = %q, want %q", path, got, want)
	}
}

func assertAtomicContractEntries(t *testing.T, directory string, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Fatalf("entries in %s = %v, want %v", directory, entries, want)
	}
	for i, entry := range entries {
		if entry.Name() != want[i] {
			t.Errorf("entry %d in %s = %q, want %q", i, directory, entry.Name(), want[i])
		}
	}
}

type atomicContractReadCloser struct {
	reader io.Reader
	reads  int
	closes int
}

func (reader *atomicContractReadCloser) Read(data []byte) (int, error) {
	reader.reads++
	return reader.reader.Read(data)
}

func (reader *atomicContractReadCloser) Close() error {
	reader.closes++
	return nil
}
