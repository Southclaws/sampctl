package rook

// ensure.go contains functions to install, update and validate dependencies of a project.

// EnsureProject will load a project's json file and make sure all the necessary dependencies are
// present in the sibling directory `vendor/`.
func EnsureProject(dir string) (err error) {
	return
}

// EnsurePackage will make sure a vendor directory contains the specified package.
// If the package is not present, it will clone it at the correct version tag, sha1 or HEAD
// If the package is present, it will ensure the directory contains the correct version
func EnsurePackage(vendorDirectory string, pkg Package) (err error) {
	return
}

// Get will retrieve package from GitHub and place it in the specified directory.
func Get(dir string, pkg Package) (err error) {
	return
}

// CheckoutVersion will make sure a package directory (git repo) is pointing to the correct commit
// that matches the version for the dependency.
func CheckoutVersion(dir string, pkg Package) (err error) {
	return
}
