package fs

import (
	"os"
	"path/filepath"
)

func Join(parts ...string) string {
	return filepath.Join(parts...)
}

func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// MustAbs wraps Abs and panics on error.
func MustAbs(path string) string {
	abs, err := Abs(path)
	if err != nil {
		panic(err)
	}
	return abs
}

// Rel makes a path relative to the current working directory.
// If it fails, it returns the original path.
func Rel(path string) string {
	rel, err := filepath.Rel(".", path)
	if err != nil {
		return path
	}
	return rel
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}
