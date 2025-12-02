package pawnpackage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"

	"dario.cat/mergo"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/util"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
	"github.com/Southclaws/sampctl/src/pkg/runtime/run"
	"github.com/Southclaws/sampctl/src/resource"
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
// nolint:lll
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
	versioning.DependencyMeta `yaml:"-,inline"`

	// Metadata, set by the package author to describe the package
	Contributors []string `json:"contributors,omitempty" yaml:"contributors,omitempty"` // list of contributors
	Website      string   `json:"website,omitempty" yaml:"website,omitempty"`           // website or forum topic associated with the package

	// Functional, set by the package author to declare relevant files and dependencies
	Entry                 string                        `json:"entry,omitempty" yaml:"entry,omitempty"`                                     // entry point script to compile the project
	Output                string                        `json:"output,omitempty" yaml:"output,omitempty"`                                   // output amx file
	Dependencies          []versioning.DependencyString `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`                       // list of packages that the package depends on
	Development           []versioning.DependencyString `json:"dev_dependencies,omitempty" yaml:"dev_dependencies,omitempty"`               // list of packages that only the package builds depend on
	Local                 bool                          `json:"local,omitempty" yaml:"local,omitempty"`                                     // run package in local dir instead of in a temporary runtime
	Runtime               *run.Runtime                  `json:"runtime,omitempty" yaml:"runtime,omitempty"`                                 // runtime configuration
	Runtimes              []*run.Runtime                `json:"runtimes,omitempty" yaml:"runtimes,omitempty"`                               // multiple runtime configurations
	Build                 *build.Config                 `json:"build,omitempty" yaml:"build,omitempty"`                                     // build configuration
	Builds                []*build.Config               `json:"builds,omitempty" yaml:"builds,omitempty"`                                   // multiple build configurations
	IncludePath           string                        `json:"include_path,omitempty" yaml:"include_path,omitempty"`                       // include path within the repository, so users don't need to specify the path explicitly
	Resources             []resource.Resource           `json:"resources,omitempty" yaml:"resources,omitempty"`                             // list of additional resources associated with the package
	ExtractIgnorePatterns []string                      `json:"extract_ignore_patterns,omitempty" yaml:"extract_ignore_patterns,omitempty"` // patterns of files to skip when extracting plugin archives
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
	//nolint:lll
	pkg.Site, pkg.User, pkg.Repo, pkg.Path, pkg.Tag, pkg.Branch, pkg.Commit = dep.Site, dep.User, dep.Repo, dep.Path, dep.Tag, dep.Branch, dep.Commit
	return
}

// PackageFromDir attempts to parse a pawn.json or pawn.yaml file from a directory
func PackageFromDir(dir string) (pkg Package, err error) {
	packageDefinitions := []string{
		filepath.Join(dir, "pawn.json"),
		filepath.Join(dir, "pawn.yaml"),
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
		print.Verb("no package definition file (pawn.{json|yaml})")
		return
	}

	// Read the file
	data, err := ioutil.ReadFile(packageDefinition)
	if err != nil {
		err = errors.Wrapf(err, "failed to read configuration from '%s'", packageDefinition)
		return
	}

	pkg = Package{}
	if packageDefinitionFormat == "json" {
		err = json.Unmarshal(data, &pkg)
	} else {
		err = yaml.Unmarshal(data, &pkg)
	}
	if err != nil {
		err = errors.Wrapf(err, "failed to parse configuration from '%s'", packageDefinition)
		return
	}

	pkg.Format = packageDefinitionFormat

	return pkg, nil
}

// WriteDefinition creates a JSON or YAML file for a package object, the format depends
// on the `Format` field of the package.
func (pkg Package) WriteDefinition() (err error) {
	cleanPkg := pkg
	if cleanPkg.Runtime != nil {
		cleanPkg.Runtime = cleanRuntimeForWrite(cleanPkg.Runtime)
	}

	switch cleanPkg.Format {
	case "json":
		var contents []byte
		contents, err = json.MarshalIndent(cleanPkg, "", "\t")
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(cleanPkg.LocalPath, "pawn.json"), contents, 0o700)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.json")
		}
	case "yaml":
		var contents []byte
		contents, err = yaml.Marshal(cleanPkg)
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = ioutil.WriteFile(filepath.Join(cleanPkg.LocalPath, "pawn.yaml"), contents, 0o700)
		if err != nil {
			return errors.Wrap(err, "failed to write pawn.yaml")
		}
	default:
		err = errors.New("package has no format associated with it")
	}

	return
}

// cleanRuntimeForWrite creates a copy of Runtime with default values set to nil
// This prevents configor-applied defaults from being serialized to the config file
func cleanRuntimeForWrite(rt *run.Runtime) *run.Runtime {
	if rt == nil {
		return nil
	}

	clean := &run.Runtime{
		WorkingDir:    rt.WorkingDir,
		Platform:      rt.Platform,
		Container:     rt.Container,
		AppVersion:    rt.AppVersion,
		PluginDeps:    rt.PluginDeps,
		Format:        rt.Format,
		Name:          rt.Name,
		Version:       rt.Version,
		Echo:          rt.Echo,
		Gamemodes:     rt.Gamemodes,
		Filterscripts: rt.Filterscripts,
		Plugins:       rt.Plugins,
	}

	if rt.Mode != "" && rt.Mode != "server" {
		clean.Mode = rt.Mode
	}

	if rt.RuntimeType != "" {
		clean.RuntimeType = rt.RuntimeType
	}

	if !rt.RootLink {
		clean.RootLink = rt.RootLink
	}

	defaults := GetRuntimeDefaultValues()

	if rt.RCONPassword != nil && *rt.RCONPassword != defaults.RCONPassword {
		clean.RCONPassword = rt.RCONPassword
	}
	if rt.Port != nil && *rt.Port != defaults.Port {
		clean.Port = rt.Port
	}
	if rt.Hostname != nil && *rt.Hostname != defaults.Hostname {
		clean.Hostname = rt.Hostname
	}
	if rt.MaxPlayers != nil && *rt.MaxPlayers != defaults.MaxPlayers {
		clean.MaxPlayers = rt.MaxPlayers
	}

	if rt.Language != nil && *rt.Language != defaults.Language && *rt.Language != "" {
		clean.Language = rt.Language
	}

	clean.Mapname = rt.Mapname
	clean.Weburl = rt.Weburl
	clean.GamemodeText = rt.GamemodeText
	clean.Bind = rt.Bind
	clean.Password = rt.Password
	clean.Announce = rt.Announce
	clean.LANMode = rt.LANMode
	clean.Query = rt.Query
	clean.RCON = rt.RCON
	clean.LogQueries = rt.LogQueries
	clean.Sleep = rt.Sleep
	clean.MaxNPC = rt.MaxNPC
	clean.StreamRate = rt.StreamRate
	clean.StreamDistance = rt.StreamDistance
	clean.OnFootRate = rt.OnFootRate
	clean.InCarRate = rt.InCarRate
	clean.WeaponRate = rt.WeaponRate
	clean.ChatLogging = rt.ChatLogging
	clean.Timestamp = rt.Timestamp
	clean.NoSign = rt.NoSign
	clean.LogTimeFormat = rt.LogTimeFormat
	clean.MessageHoleLimit = rt.MessageHoleLimit
	clean.MessagesLimit = rt.MessagesLimit
	clean.AcksLimit = rt.AcksLimit
	clean.PlayerTimeout = rt.PlayerTimeout
	clean.MinConnectionTime = rt.MinConnectionTime
	clean.LagCompmode = rt.LagCompmode
	clean.ConnseedTime = rt.ConnseedTime
	clean.DBLogging = rt.DBLogging
	clean.DBLogQueries = rt.DBLogQueries

	return clean
}

// RuntimeDefaults holds the default values that configor applies to Runtime fields
type RuntimeDefaults struct {
	Port         int
	RCONPassword string
	Hostname     string
	MaxPlayers   int
	Language     string
}

// GetRuntimeDefaultValues returns the default values that configor applies
func GetRuntimeDefaultValues() RuntimeDefaults {
	return RuntimeDefaults{
		Port:         8192,
		RCONPassword: "",
		Hostname:     "SA-MP Server",
		MaxPlayers:   50,
		Language:     "-",
	}
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
func GetRemotePackage(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	pkg, err = PackageFromOfficialRepo(ctx, client, meta)
	if err != nil {
		return PackageFromRepo(ctx, client, meta)
	}
	return
}

// PackageFromRepo attempts to get a package from the given package definition's public repo
func PackageFromRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	repo, _, err := client.Repositories.Get(ctx, meta.User, meta.Repo)
	if err != nil {
		return
	}
	var resp *http.Response

	resp, err = http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.json",
		meta.User, meta.Repo, *repo.DefaultBranch,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return packageFromJSONResponse(resp, meta)
	}

	resp, err = http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/pawn.yaml",
		meta.User, meta.Repo, *repo.DefaultBranch,
	))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return packageFromYAMLResponse(resp, meta)
	}

	return pkg, errors.Wrap(err, "package does not point to a valid remote package")
}

// PackageFromOfficialRepo attempts to get a package from the sampctl/plugins official repository
// this repo is mainly only used for testing plugins before being PR'd into their respective repos.
func PackageFromOfficialRepo(
	ctx context.Context,
	client *github.Client,
	meta versioning.DependencyMeta,
) (pkg Package, err error) {
	resp, err := http.Get(fmt.Sprintf(
		"https://raw.githubusercontent.com/sampctl/plugins/master/%s-%s.json",
		meta.User, meta.Repo,
	))
	if err != nil {
		err = errors.Wrapf(err, "failed to get plugin '%s' from official repo", meta)
		return
	}
	defer resp.Body.Close()
	return packageFromJSONResponse(resp, meta)
}

func packageFromJSONResponse(resp *http.Response, meta versioning.DependencyMeta) (pkg Package, err error) {
	if resp.StatusCode != 200 {
		err = errors.Errorf("plugin '%s' does not exist in official repo", meta)
		return
	}
	err = json.NewDecoder(resp.Body).Decode(&pkg)
	if err != nil {
		err = errors.Wrapf(err, "failed to decode plugin package '%s'", meta)
		return
	}
	return
}

func packageFromYAMLResponse(resp *http.Response, meta versioning.DependencyMeta) (pkg Package, err error) {
	if resp.StatusCode != 200 {
		err = errors.Errorf("plugin '%s' does not exist in official repo", meta)
		return
	}
	err = yaml.NewDecoder(resp.Body).Decode(&pkg)
	if err != nil {
		err = errors.Wrapf(err, "failed to decode plugin package '%s'", meta)
		return
	}
	return
}

// GetBuildConfig returns a matching build by name from the package build list. If no name is
// specified, the first build is returned. If the package has no build definitions, a default
// configuration is returned.
func (pkg Package) GetBuildConfig(name string) (config *build.Config) {
	def := build.Default()

	// if there are no builds at all, use default
	if len(pkg.Builds) == 0 && pkg.Build == nil {
		return def
	}

	// if the user did not specify a specific build config, use the first
	// otherwise, search for a matching config by name
	if name == "" {
		if pkg.Build != nil {
			config = pkg.Build
		} else {
			config = pkg.Builds[0]

			if pkg.Build != nil {
				_ = mergo.Merge(&config, pkg.Builds[0])
			}
		}
	} else {
		for _, cfg := range pkg.Builds {
			if cfg.Name == name {
				config = cfg

				if pkg.Build != nil {
					_ = mergo.Merge(config, pkg.Build)
				}

				break
			}
		}
	}

	if config == nil {
		if pkg.Build != nil {
			print.Warn("Build doesn't exist, defaulting to main build")
			config = pkg.Build
		} else {
			print.Warn("No build config called:", name, "using default")
			config = def
		}
	}

	if config.Version != "" {
		config.Compiler.Version = string(config.Version)
	}

	if config.Compiler.Version == "" {
		config.Compiler.Version = def.Compiler.Version
	}

	if len(config.Args) == 0 {
		config.Args = def.Args
	}

	return config
}

// GetRuntimeConfig returns a matching runtime config by name from the package
// runtime list. If no name is specified, the first config is returned. If the
// package has no configurations, a default configuration is returned.
func (pkg Package) GetRuntimeConfig(name string) (config run.Runtime, err error) {
	if len(pkg.Runtimes) > 0 {
		// if the user did not specify a specific runtime config, use the first
		// otherwise, search for a matching config by name
		if name == "" {
			config = *pkg.Runtimes[0]

			if pkg.Runtime != nil {
				_ = mergo.Merge(&config, pkg.Runtime)
			}

			print.Verb(pkg, "searching", name, "in 'runtimes' list")
		} else {
			print.Verb(pkg, "using first config from 'runtimes' list")
			found := false
			for _, cfg := range pkg.Runtimes {
				if cfg.Name == name {
					config = *cfg
					found = true

					if pkg.Runtime != nil {
						_ = mergo.Merge(&config, pkg.Runtime)
					}

					break
				}
			}
			if !found {
				err = errors.Errorf("no runtime config '%s'", name)
				return
			}
		}
	} else if pkg.Runtime != nil {
		print.Verb(pkg, "using config from 'runtime' field")
		config = *pkg.Runtime
	} else {
		print.Verb(pkg, "using default config")
		config = run.Runtime{}
	}
	run.ApplyRuntimeDefaults(&config)

	return config, nil
}
