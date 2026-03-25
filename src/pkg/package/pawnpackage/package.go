package pawnpackage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/Southclaws/sampctl/src/pkg/build/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
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
	// containing a `pawn.json`/`pawn.yaml` file and a set of Pawn source code files.
	// If this field is not set, then the Package is just an in-memory pointer to a remote package.
	LocalPath string `json:"-" yaml:"-"`
	// The vendor directory - for simple packages with no sub-dependencies, this is simply
	// `<local>/dependencies` but for nested dependencies, this needs to be set.
	Vendor string `json:"-" yaml:"-"`
	// format stores the original format of the package definition file, either `json` or `yaml`
	Format string `json:"-" yaml:"-"`

	// DependencyMeta is retained for backward compatibility with older tests and call sites.
	// New code should prefer Dependency() / SetDependencyMeta().
	DependencyMeta versioning.DependencyMeta `json:"-" yaml:"-"`
	Site           string                    `json:"site,omitempty" yaml:"site,omitempty"`
	User           string                    `json:"user" yaml:"user"`
	Repo           string                    `json:"repo" yaml:"repo"`
	Path           string                    `json:"path,omitempty" yaml:"path,omitempty"`
	Tag            string                    `json:"tag,omitempty" yaml:"tag,omitempty"`
	Branch         string                    `json:"branch,omitempty" yaml:"branch,omitempty"`
	Commit         string                    `json:"commit,omitempty" yaml:"commit,omitempty"`
	SSH            string                    `json:"ssh,omitempty" yaml:"ssh,omitempty"`
	Scheme         string                    `json:"scheme,omitempty" yaml:"scheme,omitempty"`

	// Metadata, set by the package author to describe the package
	Contributors []string `json:"contributors,omitempty" yaml:"contributors,omitempty"` // list of contributors
	Website      string   `json:"website,omitempty" yaml:"website,omitempty"`           // website or forum topic associated with the package

	// Functional, set by the package author to declare relevant files and dependencies
	Preset                string                        `json:"preset,omitempty" yaml:"preset,omitempty"`                                   // package preset controlling default runtime/compiler (samp, openmp)
	Experimental          *ExperimentalConfig           `json:"experimental,omitempty" yaml:"experimental,omitempty"`                       // experimental feature flags
	Entry                 string                        `json:"entry,omitempty" yaml:"entry,omitempty"`                                     // entry point script to compile the project
	Output                string                        `json:"output,omitempty" yaml:"output,omitempty"`                                   // output amx file
	Dependencies          []versioning.DependencyString `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`                       // list of packages that the package depends on
	Development           []versioning.DependencyString `json:"dev_dependencies,omitempty" yaml:"dev_dependencies,omitempty"`               // list of packages that only the package builds depend on
	Local                 *bool                         `json:"local,omitempty" yaml:"local,omitempty"`                                     // run package in local dir instead of in a temporary runtime (nil = inferred)
	Runtime               *run.Runtime                  `json:"runtime,omitempty" yaml:"runtime,omitempty"`                                 // runtime configuration
	Runtimes              []*run.Runtime                `json:"runtimes,omitempty" yaml:"runtimes,omitempty"`                               // multiple runtime configurations
	Build                 *build.Config                 `json:"build,omitempty" yaml:"build,omitempty"`                                     // build configuration
	Builds                []*build.Config               `json:"builds,omitempty" yaml:"builds,omitempty"`                                   // multiple build configurations
	IncludePath           string                        `json:"include_path,omitempty" yaml:"include_path,omitempty"`                       // include path within the repository, so users don't need to specify the path explicitly
	Resources             []resource.Resource           `json:"resources,omitempty" yaml:"resources,omitempty"`                             // list of additional resources associated with the package
	ExtractIgnorePatterns []string                      `json:"extract_ignore_patterns,omitempty" yaml:"extract_ignore_patterns,omitempty"` // patterns of files to skip when extracting plugin archives
}

// ExperimentalConfig contains experimental feature flags for package behavior.
type ExperimentalConfig struct {
	BuildFile *bool `json:"build_file,omitempty" yaml:"build_file,omitempty"` // generate a build-time include file with constants
}

// ExperimentalFlags resolves experimental config, returning an empty config when unset.
func (pkg Package) ExperimentalFlags() *ExperimentalConfig {
	if pkg.Experimental != nil {
		return pkg.Experimental
	}
	return &ExperimentalConfig{}
}

// BuildFileEnabled reports whether the build-time include file feature is enabled.
func (cfg ExperimentalConfig) BuildFileEnabled() bool {
	if cfg.BuildFile == nil {
		return true
	}

	return *cfg.BuildFile
}

func (pkg Package) effectivePreset() string {
	preset := strings.ToLower(strings.TrimSpace(pkg.Preset))
	if preset != "" {
		return preset
	}

	if pkg.Runtime != nil && pkg.Runtime.Version != "" {
		if run.DetectRuntimeType(pkg.Runtime.Version) == run.RuntimeTypeOpenMP {
			return "openmp"
		}
	}
	for _, rt := range pkg.Runtimes {
		if rt != nil && rt.Version != "" {
			if run.DetectRuntimeType(rt.Version) == run.RuntimeTypeOpenMP {
				return "openmp"
			}
		}
	}

	return "samp"
}

// Dependency returns the current dependency metadata for this package.
func (pkg Package) Dependency() versioning.DependencyMeta {
	meta := pkg.DependencyMeta
	if pkg.Site != "" {
		meta.Site = pkg.Site
	}
	if pkg.User != "" {
		meta.User = pkg.User
	}
	if pkg.Repo != "" {
		meta.Repo = pkg.Repo
	}
	if pkg.Path != "" {
		meta.Path = pkg.Path
	}
	if pkg.Tag != "" {
		meta.Tag = pkg.Tag
	}
	if pkg.Branch != "" {
		meta.Branch = pkg.Branch
	}
	if pkg.Commit != "" {
		meta.Commit = pkg.Commit
	}
	if pkg.SSH != "" {
		meta.SSH = pkg.SSH
	}
	if pkg.Scheme != "" {
		meta.Scheme = pkg.Scheme
	}

	return meta
}

// SetDependencyMeta applies dependency metadata to the package and its compatibility field.
func (pkg *Package) SetDependencyMeta(meta versioning.DependencyMeta) {
	pkg.DependencyMeta = meta
	pkg.Site = meta.Site
	pkg.User = meta.User
	pkg.Repo = meta.Repo
	pkg.Path = meta.Path
	pkg.Tag = meta.Tag
	pkg.Branch = meta.Branch
	pkg.Commit = meta.Commit
	pkg.SSH = meta.SSH
	pkg.Scheme = meta.Scheme
}

func (pkg Package) String() string {
	return fmt.Sprint(pkg.Dependency())
}

// Validate checks a package for missing fields
func (pkg Package) Validate() (err error) {
	if pkg.Entry == pkg.Output && pkg.Entry != "" && pkg.Output != "" {
		return errors.New("package entry and output point to the same file")
	}

	return
}

// EffectiveLocal resolves whether this package should run inside the working directory.
func (pkg Package) EffectiveLocal() bool {
	if pkg.Local != nil {
		return *pkg.Local
	}

	return pkg.Parent
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
	pkg.SetDependencyMeta(dep)
	return
}

// WriteDefinition creates a JSON or YAML file for a package object, the format depends
// on the `Format` field of the package.
func (pkg Package) WriteDefinition() (err error) {
	cleanPkg := pkg
	cleanPkg.SetDependencyMeta(cleanPkg.Dependency())
	if cleanPkg.Runtime != nil {
		cleanPkg.Runtime = run.CloneWithoutDefaults(cleanPkg.Runtime)
	}

	switch cleanPkg.Format {
	case "json":
		var contents []byte
		contents, err = json.MarshalIndent(cleanPkg, "", "\t")
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = writeDefinitionFile(filepath.Join(cleanPkg.LocalPath, "pawn.json"), "pawn.json", contents)
	case "yaml":
		var contents []byte
		contents, err = yaml.Marshal(cleanPkg)
		if err != nil {
			return errors.Wrap(err, "failed to encode package metadata")
		}
		err = writeDefinitionFile(filepath.Join(cleanPkg.LocalPath, "pawn.yaml"), "pawn.yaml", contents)
	default:
		err = errors.New("package has no format associated with it")
	}

	return
}

// GetBuildConfig returns a matching build by name from the package build list. If no name is
// specified, the first build is returned. If the package has no build definitions, a default
// configuration is returned.
func (pkg Package) GetBuildConfig(name string) (config *build.Config) {
	def := build.Default()
	preset := pkg.effectivePreset()
	noBuildDefs := len(pkg.Builds) == 0 && pkg.Build == nil
	if noBuildDefs {
		switch preset {
		case "samp", "openmp":
			def.Compiler.Preset = preset
		}
	}

	// if there are no builds at all, use default
	if len(pkg.Builds) == 0 && pkg.Build == nil {
		return def
	}

	switch {
	case name == "" && pkg.Build != nil:
		config = pkg.Build
	default:
		if selected, ok := selectConfig(name, pkg.Builds, func(cfg *build.Config) string {
			return cfg.Name
		}); ok {
			config = selected

			if pkg.Build != nil && config != pkg.Build {
				_ = mergo.Merge(config, pkg.Build)
			}
		}
	}

	if config == nil {
		if pkg.Build != nil {
			config = pkg.Build
		} else {
			print.Warn("No build config called:", name, "using default")
			config = def
		}
	}

	if config.Compiler.Path == "" {
		if config.Version != "" {
			config.Compiler.Version = string(config.Version)
		}

		if config.Compiler.Version == "" {
			config.Compiler.Version = def.Compiler.Version
		}
	}

	if len(config.Args) == 0 {
		config.Args = def.Args
	}

	if config.Compiler.Path == "" &&
		config.Compiler.Site == "" && config.Compiler.User == "" && config.Compiler.Repo == "" && config.Compiler.Version == "" &&
		config.Compiler.Preset == "" {
		switch preset {
		case "samp", "openmp":
			config.Compiler.Preset = preset
		}
	}

	return config
}

// GetRuntimeConfig returns a matching runtime config by name from the package
// runtime list. If no name is specified, the first config is returned. If the
// package has no configurations, a default configuration is returned.
func (pkg Package) GetRuntimeConfig(name string) (config run.Runtime, err error) {
	if selected, ok := selectConfig(name, pkg.Runtimes, func(cfg *run.Runtime) string {
		return cfg.Name
	}); ok {
		config = *selected

		if pkg.Runtime != nil {
			_ = mergo.Merge(&config, pkg.Runtime, mergo.WithOverride)
		}

		if name == "" {
			print.Verb(pkg, "searching", name, "in 'runtimes' list")
		} else {
			print.Verb(pkg, "using first config from 'runtimes' list")
		}
	} else if len(pkg.Runtimes) > 0 && name != "" {
		err = errors.Errorf("no runtime config '%s'", name)
		return
	} else if pkg.Runtime != nil {
		print.Verb(pkg, "using config from 'runtime' field")
		config = *pkg.Runtime
	} else {
		print.Verb(pkg, "using default config")
		config = run.Runtime{}
	}

	if config.Version == "" {
		switch pkg.effectivePreset() {
		case "openmp":
			config.Version = "openmp"
		case "samp":
			config.Version = "0.3.7"
		}
	}
	run.ApplyRuntimeDefaults(&config)

	return config, nil
}
