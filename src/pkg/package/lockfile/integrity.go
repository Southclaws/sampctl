package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// IntegrityPrefix is the prefix for SHA256 integrity hashes
const IntegrityPrefix = "sha256:"

// CalculateDirectoryIntegrity calculates a SHA256 hash of a directory's contents.
// It hashes all files recursively, sorted by path for deterministic results.
// Only includes relevant source files (.inc, .pwn, .json, .yaml).
func CalculateDirectoryIntegrity(dir string) (string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", errors.Errorf("directory does not exist: %s", dir)
	}

	hasher := sha256.New()
	var files []string

	// Collect all relevant files
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files/folders
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Only include relevant file types for Pawn packages
		ext := strings.ToLower(filepath.Ext(path))
		if isRelevantExtension(ext) {
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}

		return nil
	})

	if err != nil {
		return "", errors.Wrap(err, "failed to walk directory")
	}

	// Sort files for deterministic hashing
	sort.Strings(files)

	// Hash each file's path and content
	for _, relPath := range files {
		fullPath := filepath.Join(dir, relPath)

		// Include the relative path in the hash for integrity
		_, err := hasher.Write([]byte(relPath))
		if err != nil {
			return "", errors.Wrap(err, "failed to hash file path")
		}

		// Hash file content
		err = hashFileContent(hasher, fullPath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash file: %s", relPath)
		}
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return IntegrityPrefix + hash, nil
}

// hashFileContent adds a file's content to the hasher
func hashFileContent(hasher io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(hasher, file)
	return err
}

// isRelevantExtension checks if a file extension is relevant for Pawn packages
func isRelevantExtension(ext string) bool {
	relevantExtensions := map[string]bool{
		".inc":  true, // Include files
		".pwn":  true, // Pawn source files
		".json": true, // Configuration files
		".yaml": true, // Configuration files
		".yml":  true, // Configuration files
		".md":   true, // Documentation
		".txt":  true, // Text files (often include licenses)
	}
	return relevantExtensions[ext]
}

// VerifyIntegrity verifies that a directory matches its recorded integrity hash
func VerifyIntegrity(dir, expectedHash string) (bool, error) {
	if expectedHash == "" {
		// No hash to verify
		return true, nil
	}

	actualHash, err := CalculateDirectoryIntegrity(dir)
	if err != nil {
		return false, errors.Wrap(err, "failed to calculate integrity hash")
	}

	matches := actualHash == expectedHash
	if !matches {
		print.Verb("integrity mismatch for", dir)
		print.Verb("expected:", expectedHash)
		print.Verb("actual:", actualHash)
	}

	return matches, nil
}

// CalculateCommitIntegrity creates a simple integrity string from commit SHA
// This is a fallback when full directory hashing is not practical
func CalculateCommitIntegrity(commitSHA string) string {
	if commitSHA == "" {
		return ""
	}
	return fmt.Sprintf("commit:%s", commitSHA)
}

// ParseIntegrity parses an integrity string and returns the type and value
func ParseIntegrity(integrity string) (integrityType, value string) {
	parts := strings.SplitN(integrity, ":", 2)
	if len(parts) != 2 {
		return "", integrity
	}
	return parts[0], parts[1]
}

// IsValidIntegrity checks if an integrity string is properly formatted
func IsValidIntegrity(integrity string) bool {
	if integrity == "" {
		return true // Empty is valid (optional field)
	}

	integrityType, value := ParseIntegrity(integrity)
	if integrityType == "" || value == "" {
		return false
	}

	switch integrityType {
	case "sha256":
		// SHA256 produces 64 hex characters
		return len(value) == 64
	case "commit":
		// Git commit SHA is 40 characters
		return len(value) == 40
	default:
		return false
	}
}