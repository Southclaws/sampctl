package runtime

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"runtime"

	"github.com/pkg/errors"
)

// Packages is a collection of Package objects for sorting
type Packages []Package

// Package represents a SA:MP server version, it stores both platform filenames and a checksum
type Package struct {
	Version       string
	Linux         string
	Win32         string
	LinuxChecksum string
	Win32Checksum string
	LinuxPaths    map[string]string
	Win32Paths    map[string]string
}

// AllPackages is a hard-coded list of packages soon to be moved to an external JSON resource
var AllPackages = []Package{
	Package{
		"0.3.8-RC1",
		"samp038svr_RC1.tar.gz",
		"samp038_svr_RC1_win32.zip",
		"",
		"",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3.7-R2-2-1",
		"samp037svr_R2-2-1.tar.gz",
		"samp037_svr_R2-2-1_win32.zip",
		"f20eb466306226274511b111edbadb1f",
		"2ce9cc6c4f322a39ed6221c72ff2233d",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3.7-R2-1",
		"samp037svr_R2-1.tar.gz",
		"samp037_svr_R2-1-1_win32.zip",
		"29b1da32c50d7dc0454fdc8237f8281a",
		"b33733969f7dc7572e154ab70011767f",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3z-R4",
		"samp03zsvr_R4.tar.gz",
		"samp03z_svr_R4_win32.zip",
		"c4aac3c696072ad009dddcdce41c5d18",
		"428f72ba4468a05498287f7bf599d075",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3z-R3",
		"samp03zsvr_R3.tar.gz",
		"samp03z_svr_R3_win32.zip",
		"964e221f6aa43c739cfe96862d4caf3b",
		"8ab95699ad15689e1c444389f4b0d99f",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3z-R2-2",
		"samp03zsvr_R2-2.tar.gz",
		"samp03z_svr_R2-2_win32.zip",
		"9f19f1df3020032a2e95e6aa93eab280",
		"9c08de894244b07feb154579a12c7782",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3z-R1",
		"samp03zsvr_R1.tar.gz",
		"samp03z_svr_R1_win32.zip",
		"60dace014f6f812e77377e24dde540af",
		"df2aa201e74a92003456c509576db8bf",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
	Package{
		"0.3z-R1-2",
		"samp03zsvr_R1-2.tar.gz",
		"samp03z_svr_R1-2_win32.zip",
		"a84f1247d6bbf1e3ecaa634ecc5e5d1d",
		"723f0a00d4f4f3dfb1e1b5fd9d36133c",
		map[string]string{
			"samp03/samp03svr": "samp03svr",
			"samp03/announce":  "announce",
			"samp03/samp-npc":  "samp-npc",
		},
		map[string]string{
			"samp-server.exe": "samp-server.exe",
			"announce.exe":    "announce.exe",
			"samp-npc.exe":    "samp-npc.exe",
		},
	},
}

func isBinary(filename string) bool {
	switch runtime.GOOS {
	case "windows":
		switch filename {
		case "samp-server.exe", "announce.exe", "samp-npc.exe":
			return true
		}
	case "linux", "darwin":
		switch filename {
		case "samp03svr", "announce", "samp-npc":
			return true
		}
	}
	return false
}

func getServerBinary() string {
	switch runtime.GOOS {
	case "windows":
		return "samp-server.exe"
	case "linux", "darwin":
		return "samp03svr"
	default:
		return ""
	}
}

func getNpcBinary() string {
	switch runtime.GOOS {
	case "windows":
		return "samp-npc.exe"
	case "linux", "darwin":
		return "samp-npc"
	default:
		return ""
	}
}

func getAnnounceBinary() string {
	switch runtime.GOOS {
	case "windows":
		return "announce.exe"
	case "linux", "darwin":
		return "announce"
	default:
		return ""
	}
}

func matchesChecksum(src, version string) (ok bool, err error) {
	pkg, err := FindPackage(version)
	if err != nil {
		return
	}

	contents, err := ioutil.ReadFile(src)
	if err != nil {
		return false, errors.Wrap(err, "failed to read server binary")
	}

	want := ""
	switch runtime.GOOS {
	case "windows":
		want = pkg.Win32Checksum
	case "linux", "darwin":
		want = pkg.LinuxChecksum
	default:
		return false, errors.New("platform not supported")
	}
	hasher := md5.New()
	_, err = hasher.Write(contents)
	if err != nil {
		return false, errors.Wrap(err, "failed to write to md5 hasher")
	}

	return hex.EncodeToString(hasher.Sum(nil)) == want, nil
}

// FindPackage returns a server resource package for the given version or nil if it's invalid
func FindPackage(version string) (pkg Package, err error) {
	for _, pkg = range AllPackages {
		if pkg.Version == version {
			return
		}
	}
	return pkg, errors.Errorf("server package for '%s' not found", version)
}
