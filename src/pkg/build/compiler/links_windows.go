//go:build windows

package compiler

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func compilerSingleLink(os.FileInfo) bool { return true }

func compilerPathSingleLink(path string, info os.FileInfo) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	current, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, current) {
		return false, errors.Join(statErr, file.Close())
	}
	var attributes windows.ByHandleFileInformation
	queryErr := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &attributes)
	return attributes.NumberOfLinks == 1, errors.Join(queryErr, file.Close())
}
