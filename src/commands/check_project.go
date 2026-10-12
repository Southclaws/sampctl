package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func checkProject(dir string) (pawnpackage.Package, diagnostic) {
	d := diagnostic{Name: "project", Status: checkSkipped, Message: "No project configuration found in this directory or its parents", Path: dir}
	root, path, err := findCheckProject(dir)
	if err != nil {
		d.Status, d.Message, d.Err = checkFailed, "Project configuration could not be discovered", err
		d.Action = "Check access to the project directory and its configuration files."
		return pawnpackage.Package{}, d
	}
	if root == "" {
		return pawnpackage.Package{}, d
	}
	d.Path, d.Status = path, checkFailed
	d.Action = "Correct the project configuration, then run `sampctl check` again."
	if path == "" {
		d.Path = root
		d.Message = "Project contains both pawn.json and pawn.yaml"
		d.Action = "Keep one project configuration file."
		return pawnpackage.Package{}, d
	}
	info, err := os.Stat(path)
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("configuration is not a regular file")
	}
	if err != nil {
		d.Message, d.Err = "Project configuration is unavailable: "+filepath.Base(path), err
		return pawnpackage.Package{}, d
	}
	data, err := os.ReadFile(path)
	if err != nil {
		d.Message, d.Err = "Project configuration could not be read: "+filepath.Base(path), err
		return pawnpackage.Package{}, d
	}
	pkg, err := pawnpackage.PackageFromDefinition(data, filepath.Ext(path)[1:])
	if err != nil {
		d.Message, d.Err = "Project configuration is malformed: "+filepath.Base(path)+jsonErrorLocation(data, err), err
		return pawnpackage.Package{}, d
	}
	if err := validateCheckProject(pkg); err != nil {
		d.Message, d.Err = "Project configuration is invalid: "+filepath.Base(path), err
		return pawnpackage.Package{}, d
	}
	pkg.LocalPath, pkg.Vendor, pkg.Parent = root, filepath.Join(root, "dependencies"), true
	d.Status, d.Message, d.Action = checkPassed, "Project configuration is valid", ""
	return pkg, d
}

func findCheckProject(dir string) (string, string, error) {
	for {
		var paths []string
		for _, name := range []string{"pawn.json", "pawn.yaml"} {
			path := filepath.Join(dir, name)
			_, err := os.Lstat(path)
			if err == nil {
				paths = append(paths, path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", "", fmt.Errorf("inspect %s: %w", path, err)
			}
		}
		if len(paths) == 1 {
			return dir, paths[0], nil
		}
		if len(paths) > 1 {
			return dir, "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

func validateCheckProject(pkg pawnpackage.Package) error {
	if err := pkg.Validate(); err != nil {
		return err
	}
	for _, cfg := range pkg.Builds {
		if cfg == nil {
			return errors.New("builds contains a null configuration")
		}
	}
	for _, cfg := range pkg.Runtimes {
		if cfg == nil {
			return errors.New("runtimes contains a null configuration")
		}
	}
	return nil
}

func jsonErrorLocation(data []byte, err error) string {
	var syntax *json.SyntaxError
	var field *json.UnmarshalTypeError
	var offset int64
	if errors.As(err, &syntax) {
		offset = syntax.Offset
	} else if errors.As(err, &field) {
		offset = field.Offset
	}
	if offset < 1 || offset > int64(len(data)) {
		return ""
	}
	prefix := data[:offset-1]
	line := bytes.Count(prefix, []byte{'\n'}) + 1
	column := len(prefix) - bytes.LastIndexByte(prefix, '\n')
	return fmt.Sprintf(" (line %d, column %d)", line, column)
}

func checkProjectPaths(pkg pawnpackage.Package) []diagnostic {
	var results []diagnostic
	seen := make(map[string]bool)
	add := func(path, label string, directory bool) {
		if path == "" {
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkg.LocalPath, path)
		}
		key := fmt.Sprintf("%t:%s", directory, path)
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, checkProjectPath(path, label, directory))
	}
	add(pkg.Entry, "Project entry", false)
	add(pkg.IncludePath, "Package include directory", true)
	configs := append([]*build.Config{pkg.Build}, pkg.Builds...)
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		add(cfg.Input, "Build input", false)
		add(cfg.WorkingDir, "Build working directory", true)
		for _, inc := range cfg.Includes {
			add(inc, "Build include directory", true)
		}
	}
	return results
}

func checkProjectPath(path, label string, directory bool) diagnostic {
	d := diagnostic{Name: "project path", Status: checkPassed, Message: label + " is available: " + filepath.Base(path), Path: path}
	info, err := os.Stat(path)
	if err == nil && (directory && !info.IsDir() || !directory && !info.Mode().IsRegular()) {
		err = errors.New("configured path has the wrong file type")
	}
	if err == nil {
		var f *os.File
		f, err = os.Open(path)
		if err == nil {
			err = f.Close()
		}
	}
	if err != nil {
		d.Status, d.Err = checkFailed, err
		d.Message = label + " is unavailable: " + filepath.Base(path)
		d.Action = "Check this path in the project configuration: " + path
	}
	return d
}
