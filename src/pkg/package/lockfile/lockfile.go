package lockfile

import (
	"fmt"
	"time"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

const Version = 1

const Filename = "pawn.lock"

type Lockfile struct {
	Version        int                          `json:"version" yaml:"version"`
	Generated      time.Time                    `json:"generated" yaml:"generated"`
	SampctlVersion string                       `json:"sampctl_version" yaml:"sampctl_version"`
	Dependencies   map[string]LockedDependency  `json:"dependencies" yaml:"dependencies"`
	Runtime        *LockedRuntime               `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Build          *LockedBuild                 `json:"build,omitempty" yaml:"build,omitempty"`
}

type LockedRuntime struct {
	Version     string            `json:"version" yaml:"version"`
	Platform    string            `json:"platform" yaml:"platform"`
	RuntimeType string            `json:"runtime_type" yaml:"runtime_type"`
	Files       []LockedFileInfo  `json:"files,omitempty" yaml:"files,omitempty"`
}

type LockedFileInfo struct {
	Path string `json:"path" yaml:"path"`
	Size int64  `json:"size" yaml:"size"`
	Hash string `json:"hash" yaml:"hash"`
	Mode uint32 `json:"mode" yaml:"mode"`
}

type LockedBuild struct {
	CompilerVersion string `json:"compiler_version,omitempty" yaml:"compiler_version,omitempty"`
	CompilerPreset  string `json:"compiler_preset,omitempty" yaml:"compiler_preset,omitempty"`
	Entry           string `json:"entry,omitempty" yaml:"entry,omitempty"`
	Output          string `json:"output,omitempty" yaml:"output,omitempty"`
	OutputHash      string `json:"output_hash,omitempty" yaml:"output_hash,omitempty"`
}

type LockedDependency struct {
	Constraint string   `json:"constraint" yaml:"constraint"`
	Resolved   string   `json:"resolved" yaml:"resolved"`
	Commit     string   `json:"commit" yaml:"commit"`
	Integrity  string   `json:"integrity,omitempty" yaml:"integrity,omitempty"`
	Site       string   `json:"site,omitempty" yaml:"site,omitempty"`
	User       string   `json:"user" yaml:"user"`
	Repo       string   `json:"repo" yaml:"repo"`
	Path       string   `json:"path,omitempty" yaml:"path,omitempty"`
	Branch     string   `json:"branch,omitempty" yaml:"branch,omitempty"`
	Transitive bool     `json:"transitive,omitempty" yaml:"transitive,omitempty"`
	RequiredBy []string `json:"required_by,omitempty" yaml:"required_by,omitempty"`
	Scheme     string   `json:"scheme,omitempty" yaml:"scheme,omitempty"`
	Local      string   `json:"local,omitempty" yaml:"local,omitempty"`
}

func New(sampctlVersion string) *Lockfile {
	return &Lockfile{
		Version:        Version,
		Generated:      time.Now().UTC(),
		SampctlVersion: sampctlVersion,
		Dependencies:   make(map[string]LockedDependency),
	}
}

func DependencyKey(meta versioning.DependencyMeta) string {
	if meta.Scheme != "" {
		if meta.Local != "" {
			return fmt.Sprintf("%s://local/%s", meta.Scheme, meta.Local)
		}
		return fmt.Sprintf("%s://%s/%s", meta.Scheme, meta.User, meta.Repo)
	}

	site := meta.Site
	if site == "" {
		site = "github.com"
	}
	return fmt.Sprintf("%s/%s/%s", site, meta.User, meta.Repo)
}

func (l *Lockfile) AddDependency(key string, dep LockedDependency) {
	if l.Dependencies == nil {
		l.Dependencies = make(map[string]LockedDependency)
	}
	l.Dependencies[key] = dep
}

func (l *Lockfile) GetDependency(key string) (LockedDependency, bool) {
	dep, ok := l.Dependencies[key]
	return dep, ok
}

func (l *Lockfile) HasDependency(key string) bool {
	_, ok := l.Dependencies[key]
	return ok
}

func (l *Lockfile) RemoveDependency(key string) {
	delete(l.Dependencies, key)
}

func (l *Lockfile) GetLockedMeta(meta versioning.DependencyMeta) (versioning.DependencyMeta, bool) {
	key := DependencyKey(meta)
	locked, ok := l.Dependencies[key]
	if !ok {
		return meta, false
	}

	lockedMeta := meta
	if locked.Commit != "" {
		lockedMeta.Commit = locked.Commit
		lockedMeta.Tag = ""
		lockedMeta.Branch = ""
	}

	return lockedMeta, true
}

func (l *Lockfile) IsOutdated(meta versioning.DependencyMeta) bool {
	key := DependencyKey(meta)
	locked, ok := l.Dependencies[key]
	if !ok {
		return true
	}
	constraint := getConstraint(meta)
	return locked.Constraint != constraint
}

func getConstraint(meta versioning.DependencyMeta) string {
	switch {
	case meta.Tag != "":
		return ":" + meta.Tag
	case meta.Branch != "":
		return "@" + meta.Branch
	case meta.Commit != "":
		return "#" + meta.Commit
	default:
		return ""
	}
}

func (l *Lockfile) UpdateTimestamp() {
	l.Generated = time.Now().UTC()
}

func (l *Lockfile) Validate() error {
	if l.Version == 0 {
		return fmt.Errorf("lockfile version is not set")
	}
	if l.Version > Version {
		return fmt.Errorf("lockfile version %d is newer than supported version %d", l.Version, Version)
	}
	return nil
}

func (l *Lockfile) DependencyCount() int {
	return len(l.Dependencies)
}

func (l *Lockfile) DirectDependencies() map[string]LockedDependency {
	direct := make(map[string]LockedDependency)
	for key, dep := range l.Dependencies {
		if !dep.Transitive {
			direct[key] = dep
		}
	}
	return direct
}

func (l *Lockfile) TransitiveDependencies() map[string]LockedDependency {
	transitive := make(map[string]LockedDependency)
	for key, dep := range l.Dependencies {
		if dep.Transitive {
			transitive[key] = dep
		}
	}
	return transitive
}

func (l *Lockfile) SetRuntime(version, platform, runtimeType string, files []LockedFileInfo) {
	l.Runtime = &LockedRuntime{
		Version:     version,
		Platform:    platform,
		RuntimeType: runtimeType,
		Files:       files,
	}
}

func (l *Lockfile) SetBuild(compilerVersion, compilerPreset, entry, output, outputHash string) {
	l.Build = &LockedBuild{
		CompilerVersion: compilerVersion,
		CompilerPreset:  compilerPreset,
		Entry:           entry,
		Output:          output,
		OutputHash:      outputHash,
	}
}

func (l *Lockfile) GetRuntime() *LockedRuntime {
	return l.Runtime
}

func (l *Lockfile) GetBuild() *LockedBuild {
	return l.Build
}

func (l *Lockfile) HasRuntime() bool {
	return l.Runtime != nil
}

func (l *Lockfile) HasBuild() bool {
	return l.Build != nil
}