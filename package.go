package main

// package stores a map of version strings to filenames for each OS.

// Package represents a SA:MP server version, it stores both platform filenames and a checksum
type Package struct {
	Linux         string
	Win32         string
	LinuxChecksum string
	Win32Checksum string
}

var (
	v037r221 = &Package{"samp037svr_R2-2-1.tar.gz", "samp037_svr_R2-2-1_win32.zip", "b5e0a204236e39c1fe9afdb6e61884ca", "bae97f464172341570dad84eaee5b3a7"}
	v037r21  = &Package{"samp037svr_R2-1.tar.gz", "samp037_svr_R2-1-1_win32.zip", "93705e165550c97484678236749198a4", "0084b401f2516b98ebedeea5e09262cf"}
	v03zr4   = &Package{"samp03zsvr_R4.tar.gz", "samp03z_svr_R4_win32.zip", "7e0f18d1a367a3522f306d1e71477005", "06b8a886d1e93fa434f8f270077b1b0b"}
	v03zr3   = &Package{"samp03zsvr_R3.tar.gz", "samp03z_svr_R3_win32.zip", "bfdeb1083b87a65a234935ccf32f38ee", "97d9bce57d60badcdf74e281ded6a798"}
	v03zr22  = &Package{"samp03zsvr_R2-2.tar.gz", "samp03z_svr_R2-2_win32.zip", "bc7a377ab39dd022de6b02100555347d", "eeec71bfe6431ef78316d0ebdacc734a"}
	v03zr1   = &Package{"samp03zsvr_R1.tar.gz", "samp03z_svr_R1_win32.zip", "215ffd7a4893caa91c737c05899d3ee9", "c0e3d21b5324e5bb01afbe4c1036d8b2"}
	v03zr12  = &Package{"samp03zsvr_R1-2.tar.gz", "samp03z_svr_R1-2_win32.zip", "93a24e619417c6608ff6505c02897159", "8ce35b52c2ed9c7b308a95375d06a150"}
)

// Packages is a simple version-string map to all known SA:MP packages
var Packages = map[string]*Package{
	"latest": v037r221,

	"0.3.7": v037r221,
	"0.3z":  v03zr4,

	// older versions
	"0.3.7-R2-2-1": v037r221,
	"0.3.7-R2-1":   v037r21,
	"0.3z-R4":      v03zr4,
	"0.3z-R3":      v03zr3,
	"0.3z-R2-2":    v03zr22,
	"0.3z-R1":      v03zr1,
	"0.3z-R1-2":    v03zr12,
}
