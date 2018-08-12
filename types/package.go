package types

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/jinzhu/configor"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/util"
	"github.com/Southclaws/sampctl/versioning"
)

// Package represents a definition for a Pawn package and can either be used to define a build or
// as a description of a package in a repository. This is akin to npm's package.json and combines
// a project's dependencies with a description of that project.
//
// For example, a gamemode that includes a library does not need to define the User, Repo, Version,
// Contributors and Include fields at all, it can just define the Dependencies list in order to
// build correctly.
//
// On the flip side, a library written in pure Pawn should define some contributors and a web URL
// but, being written in pure Pawn, has no dependencies.
//
// Finally, if a repository stores its package source files in a subdirectory, that directory should
// be specified in the Include field. This is common practice for plugins that store the plugin
// source code in the root and the Pawn source in a subdirectory called 'include'.
type Package struct {
	// Parent indicates that this package is a "working" package that the user has explicitly
	// created and is developing. The opposite of this would be packages that exist in the
	// `dependencies` directory that have been downloaded as a result of an Ensure.
	Parent bool `json:"-" yaml:"-"`
	// LocalPath indicates the Package object represents a local copy which is a directory
	// containing a `samp.json`/`samp.yaml` file and a set of Pawn source code files.
	// If this field is not set, then the Package is just an in-memory pointer to a remote package.
	LocalPath string `json:"-" yaml:"-"`
	// The vendor directory - for simple packages with no sub-dependencies, this is simply
	// `<local>/dependencies` but for nested dependencies, this needs to be set.
	Vendor string `json:"-" yaml:"-"`
	// format stores the original format of the package definition file, either `json` or `yaml`
	Format string `json:"-" yaml:"-"`

	// Inferred metadata, not always explicitly set via JSON/YAML but inferred from the dependency path
	versioning.DependencyMeta

	// Metadata, set by the package author to describe the package
	Contributors []string `json:"contributors,omitempty" yaml:"contributors,omitempty"` // list of contributors
	Website      string   `json:"website,omitempty" yaml:"website,omitempty"`           // website or forum topic associated with the package

	// Functional, set by the package author to declare relevant files and dependencies
	Entry        string                        `json:"entry,omitempty"`                                              // entry point script to compile the project
	Output       string                        `json:"output,omitempty"`                                             // output amx file
	Dependencies []versioning.DependencyString `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`         // list of packages that the package depends on
	Development  []versioning.DependencyString `json:"dev_dependencies,omitempty" yaml:"dev_dependencies,omitempty"` // list of packages that only the package builds depend on
	Local        bool                          `json:"local,omitempty" yaml:"local,omitempty"`                       // run package in local dir instead of in a temporary runtime
	Build        *BuildConfig                  `json:"build,omitempty" yaml:"build,omitempty"`                       // build configuration
	Builds       []*BuildConfig                `json:"builds,omitempty" yaml:"builds,omitempty"`                     // multiple build configurations
	Runtime      *Runtime                      `json:"runtime,omitempty" yaml:"runtime,omitempty"`                   // runtime configuration
	Runtimes     []*Runtime                    `json:"runtimes,omitempty" yaml:"runtimes,omitempty"`                 // multiple runtime configurations
	IncludePath  string                        `json:"include_path,omitempty" yaml:"include_path,omitempty"`         // include path within the repository, so users don't need to specify the path explicitly
	Resources    []Resource                    `json:"resources,omitempty" yaml:"resources,omitempty"`               // list of additional resources associated with the package
}

func (pkg Package) String() string {
	return fmt.Sprint(pkg.DependencyMeta)
}

// Validate checks a package for missing fields
func (pkg Package) Validate() (err error) {
	if pkg.Entry == pkg.Output && pkg.Entry != "" && pkg.Output != "" {
		return errors.New("package entry and output point to the same file")
	}

	return
}

// GetAllDependencies returns the Dependencies and the Development dependencies in one list
func (pkg Package) GetAllDependencies() (result []versioning.DependencyString) {
	result = append(result, pkg.Dependencies...)
	result = append(result, pkg.Development...)
	return
}

// PackageFromDep creates a Package object from a Dependency String
func PackageFromDep(depString versioning.DependencyString) (pkg Package, err error) {
	dep, err := depString.Explode()
	pkg.Site, pkg.User, pkg.Repo, pkg.Path, pkg.Tag, pkg.Branch, pkg.Commit = dep.Site, dep.User, dep.Repo, dep.Path, dep.Tag, dep.Branch, dep.Commit
	return
}

// PackageFromDir attempts to parse a pawn.json or pawn.yaml file from a directory
func PackageFromDir(dir string) (pkg Package, err error) {
	err = godotenv.Load(filepath.Join(dir, ".env"))
	if err != nil {
		print.Verb("could not load .env:", err)
		err = nil
	}

	packageDefinitions := []string{
		filepath.Join(dir, "pawn.json"),
		filepath.Join(dir, "pawn.yaml"),
		filepath.Join(dir, "pawn.toml"),
	}
	packageDefinition := ""
	packageDefinitionFormat := ""
	for _, configFile := range packageDefinitions {
		if util.Exists(configFile) {
			packageDefinition = configFile
			packageDefinitionFormat = filepath.Ext(configFile)[1:]
			break
		}
	}

	if packageDefinition == "" {
		err = errors.New("no package definition file (pawn.{json|yaml|toml})")
		return
	}

	cnfgr := configor.New(&configor.Config{
		Environment:          "development",
		ENVPrefix:            "SAMP",
		Debug:                os.Getenv("DEBUG") != "",
		Verbose:              os.Getenv("DEBUG") != "",
		ErrorOnUnmatchedKeys: true,
	})

	print.Verb("loading package definition", packageDefinitionFormat, "file", packageDefinition)

	// Note: configor returns weird errors on success for some dumb reason, awaiting fix upstream.
	err = cnfgr.Load(&pkg, packageDefinition)
	if err != nil {
		if strings.Contains(err.Error(), "cannot unmarshal !!seq into string") {
			err = nil
		} else {
			err = errors.Wrapf(err, "failed to load configuration from '%s'", packageDefinition)
			return
		}
	}

	pkg.Format = packageDefinitionFormat

	return
}

// WriteDefinition creates a JSON or YAML file for a package object, the format depends
// on the `Format` field of the package.
func (pkg Package) WriteDefinition() (err error) {
	switch pkg.Format {
	case "json":
		var contents []byte
		contents, err = json.MarshalIndent(pkg, "", "\t")
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(pkg.LocalPath, "pawn.json"), contents, 0755)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.json")
		}
	case "yaml":
		var contents []byte
		contents, err = yaml.Marshal(pkg)
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(pkg.LocalPath, "pawn.yaml"), contents, 0755)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.yaml")
		}
	case "toml":
		// TODO: Toml writer
		err = errors.New("toml output not supported")
	default:
		err = errors.New("package has no format associated with it")
	}

	return
}

// GetCachedPackage returns a package using the cached copy, if it exists
func GetCachedPackage(meta versioning.DependencyMeta, cacheDir string) (pkg Package, err error) {
	path := meta.CachePath(cacheDir)
	return PackageFromDir(path)
}

// GetRemotePackage attempts to get a package definition for the given dependency meta.
// It first checks the the sampctl central repository, if that fails it falls back to using the
// repository for the package itself. This means upstream changes to plugins can be first staged in
// the official central repository before being pulled to the package specific repository.
func GetRemotePackage(ctx context.Context, client *github.Client, meta versioning.DependencyMeta) (pkg Package, err error) {
	pkg, err = PackageFromOfficialRepo(ctx, client, meta)
	if err != nil {
		return PackageFromRepo(ctx, client, meta)
	}
	return
}

// PackageFromRepo attempts to get a package from the given package definition's public repo
func PackageFromRepo(ctx context.Context, client *github.Client, meta versioning.DependencyMeta) (pkg Package, err error) {
	repo, _, err := client.Repositories.Get(ctx, meta.User, meta.Repo)
	if err != nil {
		return
	}
	var resp *http.Response

	resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/pawn.json", meta.User, meta.Repo, *repo.DefaultBranch))
	if err != nil {
		return
	}
	if resp.StatusCode == 200 {
		var contents []byte
		contents, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(contents, &pkg)
		return
	}

	resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/pawn.yaml", meta.User, meta.Repo, *repo.DefaultBranch))
	if err != nil {
		return
	}
	if resp.StatusCode == 200 {
		var contents []byte
		contents, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		err = yaml.Unmarshal(contents, &pkg)
		return
	}

	err = errors.Wrap(err, "package does not point to a valid remote package")

	return
}

// PackageFromOfficialRepo attempts to get a package from the sampctl/plugins official repository
// this repo is mainly only used for testing plugins before being PR'd into their respective repos.
func PackageFromOfficialRepo(ctx context.Context, client *github.Client, meta versioning.DependencyMeta) (pkg Package, err error) {
	resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json", meta.User, meta.Repo))
	if err != nil {
		err = errors.Wrapf(err, "failed to get plugin '%s' from official repo", meta)
		return
	}

	if resp.StatusCode != 200 {
		err = errors.Errorf("plugin '%s' does not exist in official repo", meta)
		return
	}

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrapf(err, "failed to read response for plugin package '%s'", meta)
		return
	}
	err = json.Unmarshal(payload, &pkg)
	if err != nil {
		err = errors.Wrapf(err, "failed to decode plugin package '%s'", meta)
		return
	}

	return
}
