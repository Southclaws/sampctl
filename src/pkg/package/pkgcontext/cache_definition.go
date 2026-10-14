package pkgcontext

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

func (pcx PackageContext) readPackageFromCachedRevision(meta versioning.DependencyMeta, path string) (pawnpackage.Package, error) {
	if (meta.Tag == "" || meta.Tag == "latest") && meta.Commit == "" {
		return pawnpackage.PackageFromDir(path)
	}

	repo, err := pcx.PackageServices.repositoryStore().Open(path)
	if err != nil {
		return pawnpackage.Package{}, err
	}

	var ref *plumbing.Reference
	if meta.Tag != "" {
		ref, err = versioning.RefFromTag(repo, meta)
	} else {
		ref, err = versioning.RefFromCommit(repo, meta)
	}
	if err != nil {
		return pawnpackage.Package{}, err
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return pawnpackage.Package{}, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return pawnpackage.Package{}, err
	}

	var (
		pkg      pawnpackage.Package
		found    bool
		lastPath string
	)
	for _, definition := range []struct {
		path   string
		format string
	}{
		{path: "pawn.json", format: "json"},
		{path: "pawn.yaml", format: "yaml"},
	} {
		file, fileErr := tree.File(definition.path)
		if fileErr == object.ErrFileNotFound {
			continue
		}
		if fileErr != nil {
			return pawnpackage.Package{}, fileErr
		}
		if found {
			return pawnpackage.Package{}, errors.Errorf(
				"found both %s and %s. Keep one package definition file",
				lastPath,
				definition.path,
			)
		}
		contents, contentErr := file.Contents()
		if contentErr != nil {
			return pawnpackage.Package{}, contentErr
		}

		pkg, err = pawnpackage.PackageFromDefinition([]byte(contents), definition.format)
		if err != nil {
			return pawnpackage.Package{}, err
		}
		found = true
		lastPath = definition.path
	}

	return pkg, nil
}
