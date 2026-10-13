package fs

// SyncDir flushes directory changes where the filesystem supports it.
func SyncDir(string) error {
	// Windows does not support syncing directory handles through os.File.Sync.
	return nil
}
