package lockfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

// IntegrityPrefix identifies directory integrity hashes.
const IntegrityPrefix = "sha256:"

// CalculateDirectoryIntegrity hashes the relevant source and configuration files in dir.
func CalculateDirectoryIntegrity(dir string) (string, error) {
	return CalculateDirectoryIntegrityContext(context.Background(), dir)
}

// CalculateDirectoryIntegrityContext hashes relevant regular files in dir in path order.
func CalculateDirectoryIntegrityContext(ctx context.Context, dir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := resolveIntegrityDirectory(dir)
	if err != nil {
		return "", err
	}
	files, err := integritySourceFiles(ctx, root)
	if err != nil {
		return "", fmt.Errorf("walk integrity sources: %w", err)
	}
	hasher := sha256.New()
	for _, relPath := range files {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte(relPath)); err != nil {
			return "", errors.Wrap(err, "failed to hash file path")
		}
		if err := hashFileContentContext(ctx, hasher, filepath.Join(root, relPath)); err != nil {
			return "", errors.Wrapf(err, "failed to hash file: %s", relPath)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	return IntegrityPrefix + hash, nil
}

func integritySourceFiles(ctx context.Context, dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") || !isRelevantExtension(strings.ToLower(filepath.Ext(path))) {
			return nil
		}
		if _, err := integrityRegularFile(path); err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func hashFileContentContext(ctx context.Context, hasher io.Writer, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	original, err := integrityRegularFile(path)
	if err != nil {
		return err
	}
	flags := os.O_RDONLY
	if runtime.GOOS != "windows" {
		// A file replaced by a FIFO must not block before its mode is checked.
		flags |= syscall.O_NONBLOCK
	}
	file, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, file.Close()) }()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(original, opened) {
		return fmt.Errorf("integrity source %q changed during hashing", path)
	}
	return copyIntegrityContent(ctx, hasher, file)
}

func copyIntegrityContent(ctx context.Context, destination io.Writer, source io.Reader) error {
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := source.Read(buffer)
		if err := ctx.Err(); err != nil {
			return err
		}
		if n > 0 {
			written, err := destination.Write(buffer[:n])
			if err != nil {
				return err
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr != nil {
			if stderrors.Is(readErr, io.EOF) {
				return ctx.Err()
			}
			return readErr
		}
	}
}

func integrityRegularFile(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("integrity source %q is not a regular file", path)
	}
	return info, nil
}

func resolveIntegrityDirectory(dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("access integrity directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("integrity path %q is not a directory", dir)
	}
	return filepath.EvalSymlinks(dir)
}

func isRelevantExtension(ext string) bool {
	relevantExtensions := map[string]bool{
		".inc":  true,
		".pwn":  true,
		".json": true,
		".yaml": true,
		".yml":  true,
	}
	return relevantExtensions[ext]
}

// VerifyIntegrity compares a directory or repository with a recorded integrity value.
func VerifyIntegrity(dir, expectedHash string) (bool, error) {
	return VerifyIntegrityContext(context.Background(), dir, expectedHash)
}

// VerifyIntegrityContext verifies recorded directory or commit integrity without changing files.
func VerifyIntegrityContext(ctx context.Context, dir, expectedHash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if expectedHash == "" {
		return true, nil
	}

	integrityType, value := ParseIntegrity(expectedHash)
	switch integrityType {
	case "commit":
		return verifyCommitIntegrity(ctx, dir, value)
	case "sha256", "":
		// Continue with directory hashing below.
	default:
		return false, errors.Errorf("unsupported integrity type: %s", integrityType)
	}

	actualHash, err := CalculateDirectoryIntegrityContext(ctx, dir)
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

func verifyCommitIntegrity(ctx context.Context, dir, expectedCommit string) (bool, error) {
	repo, err := OpenRepositoryContext(ctx, dir)
	if err != nil {
		return false, errors.Wrap(err, "failed to open repository")
	}

	head, err := repo.Head()
	if err != nil {
		return false, errors.Wrap(err, "failed to read repository head")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if head.Hash().String() != expectedCommit {
		return false, nil
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return false, errors.Wrap(err, "failed to get repository worktree")
	}

	status, err := worktree.Status()
	if err != nil {
		return false, errors.Wrap(err, "failed to read repository status")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	return cleanWorktreeWithObsoleteSubmodules(ctx, repo, worktree, status)
}

// ValidateWorktreeFilesContext rejects special files in a worktree and its Git metadata.
// The contents of nested worktree symlinked directories are excluded.
func ValidateWorktreeFilesContext(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := resolveIntegrityDirectory(dir)
	if err != nil {
		return err
	}
	if err := validateGitMetadataFilesContext(ctx, dir); err != nil {
		return fmt.Errorf("inspect git metadata: %w", err)
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path != root && info.Name() == ".git" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() || info.Mode().IsRegular() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := os.Stat(path)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if resolved.IsDir() || resolved.Mode().IsRegular() {
				return nil
			}
		}
		return fmt.Errorf("repository worktree file %q is not a regular file or directory", path)
	})
}

// CalculateCommitIntegrity formats a commit revision as an integrity value.
func CalculateCommitIntegrity(commitSHA string) string {
	if commitSHA == "" {
		return ""
	}
	return fmt.Sprintf("commit:%s", commitSHA)
}

// ParseIntegrity separates an integrity type from its value.
func ParseIntegrity(integrity string) (integrityType, value string) {
	parts := strings.SplitN(integrity, ":", 2)
	if len(parts) != 2 {
		return "", integrity
	}
	return parts[0], parts[1]
}

// IsValidIntegrity reports whether integrity uses a supported type and value length.
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
