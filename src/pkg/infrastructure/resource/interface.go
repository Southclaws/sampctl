// Package resource provides a unified interface for handling different types of dependencies
// including Pawn libraries, server binaries, plugins, compilers, and arbitrary files.
package resource

import (
	"context"
)

// Resource represents a unified interface for all types of dependencies in sampctl.
// This interface allows the same dependency resolution algorithm to be used for:
// - Pawn libraries
// - Pawn entry scripts  
// - Server binaries
// - Server plugins
// - Arbitrary files (GPS data, GeoIP data, Heightmap data, etc)
// - Pawn compilers
// - Rust-based plugins
// - Lua scripts
//
// This works for Git repositories, GitHub release assets and arbitrary HTTP file downloads.
type Resource interface {
	// Version returns the resource version
	Version() (version string)
	
	// Cached checks if the resource is cached and returns the path if present
	Cached(version string) (is bool, path string)
	
	// Ensure acquires the resource if necessary, downloading and caching it
	Ensure(ctx context.Context, version, path string) (err error)
	
	// Type returns the resource type for identification
	Type() ResourceType
	
	// Identifier returns a unique identifier for this resource
	Identifier() string
}

// ResourceType represents the different types of resources
type ResourceType string

const (
	// ResourceTypePawnLibrary represents a Pawn include library
	ResourceTypePawnLibrary ResourceType = "pawn-library"
	
	// ResourceTypePawnScript represents a Pawn entry script/filterscript
	ResourceTypePawnScript ResourceType = "pawn-script"
	
	// ResourceTypeServerBinary represents a SA-MP server binary
	ResourceTypeServerBinary ResourceType = "server-binary"
	
	// ResourceTypePlugin represents a server plugin (.so/.dll)
	ResourceTypePlugin ResourceType = "plugin"
	
	// ResourceTypeCompiler represents a Pawn compiler
	ResourceTypeCompiler ResourceType = "compiler"
	
	// ResourceTypeArbitraryFile represents arbitrary files (data, configs, etc)
	ResourceTypeArbitraryFile ResourceType = "arbitrary-file"
	
	// ResourceTypeRustPlugin represents a Rust-based plugin
	ResourceTypeRustPlugin ResourceType = "rust-plugin"
	
	// ResourceTypeLuaScript represents a Lua script
	ResourceTypeLuaScript ResourceType = "lua-script"
)

// ResourceManager handles operations on multiple resources
type ResourceManager interface {
	// GetResource retrieves a resource by identifier
	GetResource(identifier string) (Resource, error)
	
	// AddResource adds a resource to the manager
	AddResource(resource Resource)
	
	// ListResources lists all available resources of a given type
	ListResources(resourceType ResourceType) ([]Resource, error)
	
	// EnsureAll ensures all resources in a dependency tree
	EnsureAll(ctx context.Context, resources []Resource) error
	
	// CleanCache removes unused cached resources
	CleanCache() error
}

// ResourceFactory creates Resource instances from various sources
type ResourceFactory interface {
	// FromDependencyString creates a resource from a dependency string (e.g., "user/repo:tag")
	FromDependencyString(depString string, resourceType ResourceType) (Resource, error)
	
	// FromURL creates a resource from a direct URL
	FromURL(url string, resourceType ResourceType) (Resource, error)
	
	// FromLocal creates a resource from a local path
	FromLocal(path string, resourceType ResourceType) (Resource, error)
}
