package runtime

import (
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
)

func extractPluginArchive(
	request EnsureVersionedPluginRequest,
	filename, ext string,
	paths map[string]string,
) (_ map[string][]string, err error) {
	if _, err := runtimeFilePath(request.Dir, ".sampctl-plugin-stage"); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(request.Dir, 0o755); err != nil {
		return nil, err
	}

	stage, err := os.MkdirTemp(request.Dir, ".sampctl-plugin-")
	if err != nil {
		return nil, err
	}
	defer func() { err = stderrors.Join(err, os.RemoveAll(stage)) }()

	stagedPaths := make(map[string]string, len(paths))
	keys := make([]string, 0, len(paths))
	for source := range paths {
		keys = append(keys, source)
	}
	sort.Strings(keys)
	for index, source := range keys {
		stagedPaths[source] = fmt.Sprintf("%d/", index)
	}

	var extracted map[string]string
	switch ext {
	case ".zip":
		extracted, err = download.Unzip(filename, stage, stagedPaths)
	case ".gz":
		extracted, err = download.Untar(filename, stage, stagedPaths)
	default:
		return nil, errors.Errorf("unsupported archive format: %s", filename)
	}
	if err != nil {
		return nil, err
	}

	if err := request.Context.Err(); err != nil {
		return nil, err
	}

	targets := make(map[string][]string, len(extracted))
	type stagedFile struct {
		path   string
		target string
	}
	var stagedFiles []stagedFile
	for _, source := range keys {
		staged, ok := extracted[source]
		if !ok {
			continue
		}

		matches := []string{staged}
		if paths[source] == "" || strings.HasSuffix(paths[source], "/") {
			// The extractor returns only the last match for each pattern, but
			// directory mappings must install every file in its staging directory.
			entries, err := os.ReadDir(filepath.Dir(staged))
			if err != nil {
				return nil, err
			}
			matches = nil
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				matches = append(matches, filepath.Join(filepath.Dir(staged), entry.Name()))
			}
		}

		for _, path := range matches {
			relative, err := filepath.Rel(stage, path)
			if err != nil {
				return nil, err
			}
			if _, err := runtimeFilePath(stage, relative); err != nil {
				return nil, err
			}

			target := paths[source]
			if target == "" {
				target = filepath.Base(path)
			} else if strings.HasSuffix(target, "/") {
				target = filepath.Join(target, filepath.Base(path))
			}

			root := request.Dir
			if !filepath.IsLocal(target) {
				if request.RestrictFilesToDir {
					return nil, errors.Wrapf(os.ErrInvalid, "plugin destination %q must stay inside its installation", target)
				}
				// Explicit resource mappings may target files outside the runtime directory.
				if !filepath.IsAbs(target) {
					target = filepath.Join(root, target)
				}
				root, target = filepath.Dir(target), filepath.Base(target)
			}

			final, err := runtimeFilePath(root, target)
			if err != nil {
				return nil, err
			}
			if ignoredRuntimePlugin(final, request.IgnorePatterns) {
				continue
			}

			stagedFiles = append(stagedFiles, stagedFile{path: path, target: final})
			targets[source] = append(targets[source], final)
		}
	}

	for _, file := range stagedFiles {
		info, err := os.Lstat(file.path)
		if err != nil {
			return nil, err
		}

		if err := copyFileWithModeContext(request.Context, file.path, file.target, info.Mode()); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

func ignoredRuntimePlugin(path string, patterns []string) bool {
	if _, err := os.Lstat(path); err != nil {
		return false
	}

	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, filepath.Base(path)); err == nil && matched {
			return true
		}
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
	}

	return false
}

func detectArchiveExt(filename string) string {
	f, err := os.Open(filename)
	if err != nil {
		return ""
	}

	var b [4]byte
	_, err = f.Read(b[:])
	if errClose := f.Close(); err != nil || errClose != nil {
		return ""
	}

	if b[0] == 'P' && b[1] == 'K' {
		return ".zip"
	}

	if b[0] == 0x1f && b[1] == 0x8b {
		return ".gz"
	}

	return ""
}
