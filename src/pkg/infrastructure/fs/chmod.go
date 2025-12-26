package fs

import "os"

func ChmodAll(files map[string]string, mode os.FileMode) error {
	for _, file := range files {
		if err := os.Chmod(file, mode); err != nil {
			return err
		}
	}
	return nil
}

func IsPosixPlatform(platform string) bool {
	return platform == "linux" || platform == "darwin"
}

func ChmodAllIfPosix(platform string, files map[string]string, mode os.FileMode) error {
	if !IsPosixPlatform(platform) {
		return nil
	}
	return ChmodAll(files, mode)
}
