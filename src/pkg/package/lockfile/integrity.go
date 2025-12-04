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

const IntegrityPrefix = "sha256:"

func CalculateDirectoryIntegrity(dir string) (string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", errors.Errorf("directory does not exist: %s", dir)
	}

	hasher := sha256.New()
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

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

	sort.Strings(files)

	for _, relPath := range files {
		fullPath := filepath.Join(dir, relPath)

		_, err := hasher.Write([]byte(relPath))
		if err != nil {
			return "", errors.Wrap(err, "failed to hash file path")
		}

		err = hashFileContent(hasher, fullPath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to hash file: %s", relPath)
		}
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return IntegrityPrefix + hash, nil
}

func hashFileContent(hasher io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(hasher, file)
	return err
}

func isRelevantExtension(ext string) bool {
	relevantExtensions := map[string]bool{
		".inc":  true,
		".pwn":  true,
		".json": true,
		".yaml": true,
		".yml":  true,
		".md":   true,
		".txt":  true,
	}
	return relevantExtensions[ext]
}

func VerifyIntegrity(dir, expectedHash string) (bool, error) {
	if expectedHash == "" {
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

func CalculateCommitIntegrity(commitSHA string) string {
	if commitSHA == "" {
		return ""
	}
	return fmt.Sprintf("commit:%s", commitSHA)
}

func ParseIntegrity(integrity string) (integrityType, value string) {
	parts := strings.SplitN(integrity, ":", 2)
	if len(parts) != 2 {
		return "", integrity
	}
	return parts[0], parts[1]
}

func IsValidIntegrity(integrity string) bool {
	if integrity == "" {
		return true
	}

	integrityType, value := ParseIntegrity(integrity)
	if integrityType == "" || value == "" {
		return false
	}

	switch integrityType {
	case "sha256":
		return len(value) == 64
	case "commit":
		return len(value) == 40
	default:
		return false
	}
}