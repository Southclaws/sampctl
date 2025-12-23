package fs

import "fmt"

func EnsurePackageLayout(projectDir string, isOpenMP bool) error {
	if err := EnsureDir(Join(projectDir, "gamemodes"), PermDirShared); err != nil {
		return fmt.Errorf("failed to create gamemodes dir: %w", err)
	}
	if err := EnsureDir(Join(projectDir, "filterscripts"), PermDirShared); err != nil {
		return fmt.Errorf("failed to create filterscripts dir: %w", err)
	}
	if err := EnsureDir(Join(projectDir, "scriptfiles"), PermDirShared); err != nil {
		return fmt.Errorf("failed to create scriptfiles dir: %w", err)
	}
	if err := EnsureDir(Join(projectDir, "plugins"), PermDirShared); err != nil {
		return fmt.Errorf("failed to create plugins dir: %w", err)
	}
	if isOpenMP {
		if err := EnsureDir(Join(projectDir, "components"), PermDirShared); err != nil {
			return fmt.Errorf("failed to create components dir: %w", err)
		}
	}
	if err := EnsureDir(Join(projectDir, "npcmodes"), PermDirShared); err != nil {
		return fmt.Errorf("failed to create npcmodes dir: %w", err)
	}
	return nil
}
