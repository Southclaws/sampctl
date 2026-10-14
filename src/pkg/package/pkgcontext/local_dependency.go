package pkgcontext

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func validateLocalDependencyPath(root string, meta versioning.DependencyMeta) error {
	path := filepath.Join(root, meta.Local)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("local dependency %s is not a regular file or directory", path)
	}

	if meta.IsIncludes() && !info.IsDir() {
		return fmt.Errorf("local include dependency %s is not a directory", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	if info.IsDir() && (meta.IsPlugin() || meta.Scheme == "component") {
		pkg, err := pawnpackage.PackageFromDir(path)
		if err != nil {
			return err
		}
		return pkg.Validate()
	}

	return nil
}
