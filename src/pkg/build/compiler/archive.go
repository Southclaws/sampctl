package compiler

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
)

type compilerContextReader struct {
	ctx    context.Context
	reader io.Reader
}

type compilerArchiveError struct {
	Path string
	Err  error
}

func (e *compilerArchiveError) Error() string {
	return fmt.Sprintf("compiler archive %s: %v", e.Path, e.Err)
}
func (e *compilerArchiveError) Unwrap() error { return e.Err }

func (r *compilerContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func compilerArchiveTargets(name string, pkg download.Compiler) ([]string, error) {
	if !compilerRelativePath(name) || name == compilerCompletionFilename {
		return nil, fmt.Errorf("unsafe compiler archive path %q", name)
	}
	targets := []string{name}
	patterns := make([]string, 0, len(pkg.Paths))
	for pattern := range pkg.Paths {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		matcher, err := regexp.Compile(pattern)
		matches := name == pattern
		if err == nil {
			matches = matcher.MatchString(name)
		}
		if !matches {
			continue
		}
		target := pkg.Paths[pattern]
		if target == "" {
			target = filepath.Base(name)
		} else if strings.HasSuffix(target, "/") {
			target += filepath.Base(name)
		}
		if !compilerRelativePath(target) || target == compilerCompletionFilename {
			return nil, fmt.Errorf("unsafe compiler extraction path %q", target)
		}
		if target != name {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func visitCompilerArchive(ctx context.Context, archive string, pkg download.Compiler, visit func(string, os.FileMode, io.Reader) error) (err error) {
	if err := compilerSafePath(filepath.Dir(archive), false); err != nil {
		return err
	}
	info, statErr := os.Lstat(archive)
	if statErr != nil {
		return statErr
	}
	if !info.Mode().IsRegular() {
		return errors.New("compiler archive must be a regular file")
	}
	if pkg.Method == download.ExtractZip {
		reader, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, reader.Close()) }()
		for _, member := range reader.File {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := strings.TrimSuffix(member.Name, "/")
			if !compilerRelativePath(name) {
				return fmt.Errorf("unsafe compiler archive path %q", member.Name)
			}
			if member.FileInfo().IsDir() {
				continue
			}
			if !member.Mode().IsRegular() {
				return fmt.Errorf("compiler archive contains a nonregular file %s", member.Name)
			}
			file, err := member.Open()
			if err != nil {
				return err
			}
			visitErr := visit(name, member.Mode(), &compilerContextReader{ctx: ctx, reader: file})
			if err := errors.Join(visitErr, file.Close()); err != nil {
				return err
			}
		}
		return nil
	}
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	var compressed io.ReadCloser
	compressed, err = gzip.NewReader(file)
	if err != nil {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return errors.Join(err, seekErr)
		}
		compressed, err = zlib.NewReader(file)
		if err != nil {
			return err
		}
	}
	defer func() { err = errors.Join(err, compressed.Close()) }()
	reader := tar.NewReader(&compilerContextReader{ctx: ctx, reader: compressed})
	for {
		member, err := reader.Next()
		if errors.Is(err, io.EOF) {
			// Read the compression trailer too, including its checksum.
			_, err = io.Copy(io.Discard, &compilerContextReader{ctx: ctx, reader: compressed})
			return err
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(member.Name, "/")
		if !compilerRelativePath(name) {
			return fmt.Errorf("unsafe compiler archive path %q", member.Name)
		}
		if member.Typeflag == tar.TypeDir {
			continue
		}
		if member.Typeflag != tar.TypeReg {
			return fmt.Errorf("compiler archive contains a nonregular file %s", member.Name)
		}
		if err := visit(name, member.FileInfo().Mode(), reader); err != nil {
			return err
		}
	}
}

func compilerArchiveInventory(ctx context.Context, archive string, pkg download.Compiler) ([]compilerFile, error) {
	return extractCompilerArchive(ctx, archive, "", pkg)
}

func extractCompilerArchive(ctx context.Context, archive, stage string, pkg download.Compiler) ([]compilerFile, error) {
	files := make(map[string]compilerFile)
	err := visitCompilerArchive(ctx, archive, pkg, func(name string, mode os.FileMode, reader io.Reader) error {
		targets, err := compilerArchiveTargets(name, pkg)
		if err != nil {
			return err
		}
		hash := sha256.New()
		var output *os.File
		if stage != "" {
			path := filepath.Join(stage, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return err
			}
			output, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return err
			}
		}
		writer := io.Writer(hash)
		if output != nil {
			writer = io.MultiWriter(hash, output)
		}
		_, copyErr := io.Copy(writer, &compilerContextReader{ctx: ctx, reader: reader})
		if output != nil {
			copyErr = errors.Join(copyErr, output.Close())
		}
		if copyErr != nil {
			return copyErr
		}
		for _, target := range targets {
			if _, exists := files[target]; exists {
				return fmt.Errorf("compiler archive maps more than one file to %s", target)
			}
			permissions := os.FileMode(0o600)
			if mode.Perm()&0o111 != 0 || target == pkg.Binary {
				permissions = 0o700
			}
			files[target] = compilerFile{Path: target, Hash: hex.EncodeToString(hash.Sum(nil)), Mode: uint32(permissions)}
			if stage != "" {
				path := filepath.Join(stage, target)
				if target != name {
					if err := copyCompilerFile(ctx, filepath.Join(stage, name), path); err != nil {
						return err
					}
				}
				if err := os.Chmod(path, permissions); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	inventory := make([]compilerFile, 0, len(files))
	for _, file := range files {
		inventory = append(inventory, file)
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Path < inventory[j].Path })
	return inventory, nil
}

func copyCompilerFile(ctx context.Context, source, target string) (err error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, &compilerContextReader{ctx: ctx, reader: input})
	return errors.Join(copyErr, output.Close())
}
