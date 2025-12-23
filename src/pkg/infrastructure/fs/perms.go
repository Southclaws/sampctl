package fs

import "os"

const (
	PermDirPrivate os.FileMode = 0o700
	PermDirShared  os.FileMode = 0o755
)

const (
	PermFilePrivate os.FileMode = 0o600
	PermFileShared  os.FileMode = 0o644
	PermFileExec    os.FileMode = 0o700
	PermFileTemp    os.FileMode = 0o600
)
