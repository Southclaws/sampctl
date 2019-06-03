package types

// -
// Compiler versions
// -

// Compilers is a list of compilers for each platform
type Compilers map[string]Compiler

// Compiler represents a compiler package for a specific OS
type Compiler struct {
	Match  string            `json:"match"`  // the release asset name pattern
	Method string            `json:"method"` // the extraction method
	Binary string            `json:"binary"` // execution binary
	Paths  map[string]string `json:"paths"`  // map of files to their target locations
}

// -
// Runtime packages for server binaries
// -

// Runtimes is a collection of Package objects for sorting
type Runtimes struct {
	Aliases  map[string]string `json:"aliases"`
	Packages []RuntimePackage  `json:"packages"`
}

// RuntimePackage represents a SA:MP server version, it stores both platform filenames and a checksum
type RuntimePackage struct {
	Version       string            `json:"version"`
	Linux         string            `json:"linux"`
	Win32         string            `json:"win32"`
	LinuxChecksum string            `json:"linux_checksum"`
	Win32Checksum string            `json:"win32_checksum"`
	LinuxPaths    map[string]string `json:"linux_paths"`
	Win32Paths    map[string]string `json:"win32_paths"`
}
