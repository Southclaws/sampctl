package runtime

import (
	"context"
	stderrors "errors"
	iofs "io/fs"
	"path/filepath"
	"regexp"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
	pkgresource "github.com/Southclaws/sampctl/src/pkg/package/resource"
)

// EnsureVersionedPluginCachedRequest describes a cache lookup for a versioned plugin asset.
type EnsureVersionedPluginCachedRequest struct {
	Context  context.Context
	Meta     versioning.DependencyMeta
	Platform string
	Version  string
	CacheDir string
	NoCache  bool
	GitHub   *github.Client
}

// PluginFetchRequest describes a plugin asset fetch from the network.
type PluginFetchRequest struct {
	Context  context.Context
	GitHub   *github.Client
	Meta     versioning.DependencyMeta
	Platform string
	Version  string
	CacheDir string
}

// EnsureVersionedPluginCached ensures that a plugin exists in the cache
func EnsureVersionedPluginCached(request EnsureVersionedPluginCachedRequest) (
	filename string,
	resource *pkgresource.Resource,
	err error,
) {
	hit := false
	if !request.NoCache {
		hit, filename, resource, err = PluginFromCache(request.Meta, request.Platform, request.Version, request.CacheDir)
		if err != nil {
			err = errors.Wrapf(err, "failed to get plugin %s from cache", request.Meta)
			return
		}
	}

	if !hit {
		if !hasExplicitDependencyReference(request.Meta) {
			//nolint:lll
			print.Info(request.Meta, "downloading the latest release because no version is specified. Pin a release tag to keep the plugin version fixed.")
		}

		filename, resource, err = pluginFromNet(PluginFetchRequest{
			Context:  request.Context,
			GitHub:   request.GitHub,
			Meta:     request.Meta,
			Platform: request.Platform,
			Version:  request.Version,
			CacheDir: request.CacheDir,
		}, resource)
		if err != nil {
			err = errors.Wrapf(err, "failed to download plugin %s", request.Meta)
			return
		}
	}

	return filename, resource, nil
}

func hasExplicitDependencyReference(meta versioning.DependencyMeta) bool {
	return meta.Tag != "" || meta.Branch != "" || meta.Commit != ""
}

// PluginFromCache tries to grab a plugin asset from the cache, `hit` indicates if it was successful
func PluginFromCache(
	meta versioning.DependencyMeta,
	platform string,
	version string,
	cacheDir string,
) (hit bool, filename string, resource *pkgresource.Resource, err error) {
	print.Verb("getting plugin resource from cache", meta)

	pkg, err := pawnpackage.GetCachedPackage(meta, cacheDir)
	if err != nil {
		print.Verb("could not load cached plugin package:", err)
		err = nil
		hit = false
		return
	}

	if pkg.Format == "" {
		return
	}

	resource, err = GetResource(pkg.Resources, platform, version)
	if err != nil {
		var missing *PluginResourceNotFoundError
		if stderrors.As(err, &missing) {
			missing.Meta = meta
		}
		return
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	ghr := infraresource.NewGitHubReleaseResource(meta, matcher, infraresource.ResourceTypePlugin, nil)
	ghr.SetCacheDir(cacheDir)

	localFilename, found := cachedPackageResourceAsset(meta.CachePath(cacheDir), matcher)
	if found {
		hit = true
		return hit, localFilename, resource, nil
	}

	var ok bool
	ok, filename = ghr.Cached(meta.Tag)
	if !ok {
		return
	}

	hit = true

	return hit, filename, resource, nil
}

func cachedPackageResourceAsset(cachePath string, matcher *regexp.Regexp) (string, bool) {
	if matcher == nil || !fs.Exists(cachePath) {
		return "", false
	}

	var matched string
	err := filepath.WalkDir(cachePath, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if matcher.MatchString(filepath.Base(path)) {
			matched = path
			return iofs.SkipAll
		}

		return nil
	})
	if err != nil || matched == "" {
		return "", false
	}

	return matched, true
}

// PluginFromNet downloads a plugin from the given metadata to the cache directory
func PluginFromNet(request PluginFetchRequest) (filename string, resource *pkgresource.Resource, err error) {
	return pluginFromNet(request, nil)
}

func pluginFromNet(request PluginFetchRequest, cachedResource *pkgresource.Resource) (filename string, resource *pkgresource.Resource, err error) {
	print.Info(request.Meta, "downloading plugin resource for", request.Platform)

	resource = cachedResource
	if resource == nil {
		pkg, fetchErr := pawnpackage.GetRemotePackage(request.Context, request.GitHub, request.Meta)
		if fetchErr != nil {
			return "", nil, errors.Wrap(fetchErr, "failed to get remote package definition file")
		}

		resource, err = GetResource(pkg.Resources, request.Platform, request.Version)
		if err != nil {
			var missing *PluginResourceNotFoundError
			if stderrors.As(err, &missing) {
				missing.Meta = request.Meta
			}
			return
		}
	}

	matcher, err := regexp.Compile(resource.Name)
	if err != nil {
		err = errors.Wrap(err, "resource name is not a valid regular expression")
		return
	}

	downloader := infraresource.NewGitHubReleaseResource(request.Meta, matcher, infraresource.ResourceTypePlugin, request.GitHub)
	downloader.SetCacheDir(request.CacheDir)

	requestedVersion := request.Meta.Tag
	if requestedVersion == "" {
		requestedVersion = "latest"
	}
	if err = downloader.Ensure(request.Context, requestedVersion, ""); err != nil {
		return
	}

	actualVersion := requestedVersion
	if actualVersion == "latest" {
		actualVersion = downloader.Version()
	}
	_, filename = downloader.Cached(actualVersion)
	if filename == "" {
		err = errors.New("failed to locate downloaded asset")
		return
	}

	print.Verb(request.Meta, "downloaded", filename, "to cache")

	return filename, resource, nil
}

// GetResource searches a list of resources for one that matches the given platform
func GetResource(resources []pkgresource.Resource, platform string, version string) (*pkgresource.Resource, error) {
	if version == "" {
		version = "0.3.7"
	}

	var selected *pkgresource.Resource
	for _, resource := range resources {
		res := resource
		if res.Platform == platform && res.Version == version {
			selected = &res
			break
		}
	}

	if selected == nil {
		for _, resource := range resources {
			res := resource
			if res.Platform == platform && res.Version == "" {
				print.Verb("using the default resource for", platform, "because none matches version", version)
				selected = &res
				break
			}
		}
	}

	if selected == nil {
		return nil, &PluginResourceNotFoundError{Platform: platform, Version: version}
	}

	if err := selected.Validate(); err != nil {
		return nil, errors.Wrap(err, "plugin resource is invalid")
	}

	return selected, nil
}

// GetResourcePath returns a path where a resource should be stored given the metadata
func GetResourcePath(meta versioning.DependencyMeta) (path string) {
	tag := meta.Tag
	if tag == "" {
		tag = "latest"
	}

	return filepath.Join("plugins", meta.Repo, tag)
}
