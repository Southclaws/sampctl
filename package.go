package main

// package stores a map of version strings to filenames for each OS.

type Package struct {
	Linux string
	Win32 string
}

var Packages = map[string]Package{
	"0.3.7-R2-2-1": {"samp037svr_R2-2-1.tar.gz", "samp037_svr_R2-2-1_win32.zip"},
}
