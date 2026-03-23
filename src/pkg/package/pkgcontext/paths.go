package pkgcontext

import "path/filepath"

func packagePath(baseDir, name string) string {
	if filepath.IsAbs(name) || baseDir == "" {
		return name
	}

	return filepath.Join(baseDir, name)
}
