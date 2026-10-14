package commands

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/build"
	compilerpkg "github.com/Southclaws/sampctl/src/pkg/build/compiler"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/download"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/fs"
	infraresource "github.com/Southclaws/sampctl/src/pkg/infrastructure/resource"
	"github.com/Southclaws/sampctl/src/pkg/package/pawnpackage"
)

const checkCompilerSupportContent = "#define SUPPORT 1\n"

func TestCheckCompilerRepairsSupportingFileFromCachedArchive(t *testing.T) {
	for _, method := range []string{download.ExtractZip, download.ExtractTgz} {
		t.Run(method, func(t *testing.T) {
			pkg, opts, request, support, _ := installCheckCompilerFixture(t, method)
			marker := filepath.Join(request.Dir, ".sampctl-compiler.json")
			completion, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			before := snapshotCheckDependency(t, opts.CacheDir)

			result := checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkFailed || !result.CanFix || result.RepairAttempted || result.Err == nil {
				t.Fatalf("checkCompilers(changed support file) = %+v, want repairable failure", result)
			}

			if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
				t.Error("read-only compiler check changed cached files")
			}

			opts.Fix = true
			result = checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkRepaired || !result.RepairAttempted || !result.RepairVerified {
				t.Fatalf("checkCompilers(fix support file) = %+v, want verified repair", result)
			}

			checkCompilerReliabilityContent(t, support, checkCompilerSupportContent)
			for _, relative := range []string{"pawncc", "libpawnc.so", "bin/pawncc", "lib/libpawnc.so", "bin/pawndisasm"} {
				if info, err := os.Stat(filepath.Join(request.Dir, relative)); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
					t.Errorf("repaired compiler file %s = %v, %v, want nonempty regular file", relative, info, err)
				}
			}

			checkCompilerReliabilityContent(t, marker, string(completion))

			state, err := compilerpkg.InspectManagedCompiler(context.Background(), request)
			if err != nil || !state.Valid || state.Pending {
				t.Fatalf("InspectManagedCompiler(after fix) = %+v, %v, want valid complete installation", state, err)
			}

			before = snapshotCheckDependency(t, opts.CacheDir)
			result = checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkPassed || result.RepairAttempted {
				t.Errorf("checkCompilers(repeat fix) = %+v, want passed without repair", result)
			}

			if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
				t.Error("repeat compiler repair rewrote a valid installation")
			}
		})
	}
}

func TestCheckCompilerMissingArchiveDoesNotAttemptRepair(t *testing.T) {
	pkg, opts, _, support, archive := installCheckCompilerFixture(t, download.ExtractZip)
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := snapshotCheckDependency(t, opts.CacheDir)
	opts.Fix = true
	result := checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkFailed || result.CanFix || result.RepairAttempted {
		t.Errorf("checkCompilers(archive missing) = %+v, want failure without automatic repair", result)
	}

	if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
		t.Error("compiler check modified files without a cached archive")
	}
}

func TestCheckCompilerRepairPreservesUnknownFiles(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "empty directory"
		}

		t.Run(name, func(t *testing.T) {
			pkg, opts, request, support, _ := installCheckCompilerFixture(t, download.ExtractZip)
			unknown := filepath.Join(request.Dir, "user-notes")
			if directory {
				if err := os.Mkdir(unknown, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(unknown, []byte("user notes"), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			before := snapshotCheckDependency(t, opts.CacheDir)
			opts.Fix = true
			result := checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkFailed || result.CanFix || result.RepairAttempted {
				t.Errorf("checkCompilers(unknown %s) = %+v, want failure without replacement", name, result)
			}

			if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
				t.Errorf("compiler repair changed an installation containing an unknown %s", name)
			}
		})
	}
}

func TestCheckCompilerExternalPathIgnoresManagedArchive(t *testing.T) {
	pkg, opts, _, support, _ := installCheckCompilerFixture(t, download.ExtractZip)
	external := t.TempDir()
	writeCheckCompilerFile(t, filepath.Join(external, checkHostCompilerName()), 0o700)
	pkg.Build = &build.Config{Compiler: build.CompilerConfig{Path: external}}

	if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheBefore := snapshotCheckDependency(t, opts.CacheDir)
	externalBefore := snapshotCheckDependency(t, external)
	opts.Fix = true
	result := checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkPassed || result.CanFix || result.RepairAttempted {
		t.Errorf("checkCompilers(external compiler) = %+v, want external compiler checked without repair", result)
	}

	if got := snapshotCheckDependency(t, external); !maps.Equal(got, externalBefore) {
		t.Error("compiler check changed the external installation")
	}

	if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, cacheBefore) {
		t.Error("checking an external compiler repaired the unused managed installation")
	}
}

func TestCheckCompilerNestedPermissionRepairWithoutArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use executable permission bits")
	}

	pkg, opts, request, support, archive := installCheckCompilerFixtureWithLayout(t, download.ExtractZip, "bin/pawncc", map[string]string{
		"bin/pawncc": "bin/pawncc", "lib/libpawnc.so": "libpawnc.so",
	})
	binary := filepath.Join(request.Dir, "bin/pawncc")
	marker := filepath.Join(request.Dir, ".sampctl-compiler.json")
	completion, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(binary, 0o600); err != nil {
		t.Fatal(err)
	}

	before := snapshotCheckDependency(t, opts.CacheDir)
	result := checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkFailed || !result.CanFix || result.Path != binary {
		t.Fatalf("checkCompilers(nested permission failure) = %+v, want repairable executable permissions", result)
	}

	if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
		t.Error("read-only permission check changed cached files")
	}

	opts.Fix = true
	result = checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkRepaired || !result.RepairAttempted || !result.RepairVerified {
		t.Fatalf("checkCompilers(fix nested executable) = %+v, want verified permission repair", result)
	}

	info, err := os.Stat(binary)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("repaired nested executable = %v, %v, want recorded mode 0700", info, err)
	}

	checkCompilerReliabilityContent(t, marker, string(completion))
	checkCompilerReliabilityContent(t, support, checkCompilerSupportContent)

	before = snapshotCheckDependency(t, opts.CacheDir)
	result = checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkPassed || result.RepairAttempted {
		t.Errorf("checkCompilers(repeat nested permission fix) = %+v, want passed without repair", result)
	}

	if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
		t.Error("repeat permission fix changed cached files")
	}
}

func TestCheckCompilerPermissionAndContentChangesRequireArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use executable permission bits")
	}

	for _, cached := range []bool{false, true} {
		name := "archive missing"
		if cached {
			name = "archive cached"
		}

		t.Run(name, func(t *testing.T) {
			pkg, opts, request, support, archive := installCheckCompilerFixtureWithLayout(t, download.ExtractTgz, "bin/pawncc", map[string]string{
				"bin/pawncc": "bin/pawncc", "lib/libpawnc.so": "libpawnc.so",
			})
			binary := filepath.Join(request.Dir, "bin/pawncc")
			if !cached {
				if err := os.Remove(archive); err != nil {
					t.Fatal(err)
				}
			}

			if err := os.Chmod(binary, 0o600); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			before := snapshotCheckDependency(t, opts.CacheDir)
			result := checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkFailed || result.CanFix != cached || result.Path != request.Dir {
				t.Fatalf("checkCompilers(permission and content changes) = %+v, want full installation failure with CanFix=%v", result, cached)
			}

			if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
				t.Error("read-only compiler check changed the damaged installation")
			}

			opts.Fix = true
			result = checkCompilerReliabilityResult(t, pkg, opts)
			if cached {
				if result.Status != checkRepaired || !result.RepairVerified {
					t.Errorf("checkCompilers(full cached repair) = %+v, want verified repair", result)
				}

				checkCompilerReliabilityContent(t, support, checkCompilerSupportContent)

				info, err := os.Stat(binary)
				if err != nil || info.Mode().Perm() != 0o700 {
					t.Errorf("full repair executable = %v, %v, want recorded mode 0700", info, err)
				}
			} else {
				if result.Status != checkFailed || result.CanFix || result.RepairAttempted {
					t.Errorf("checkCompilers(content changed without archive) = %+v, want failure without chmod", result)
				}

				if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
					t.Error("compiler check changed permissions despite unrelated content corruption")
				}
			}
		})
	}
}

func TestCheckCompilerVerifiedDirectoryMappingPasses(t *testing.T) {
	pkg, opts, _, _, _ := installCheckCompilerFixtureWithLayout(t, download.ExtractZip, "bin/pawncc", map[string]string{
		"bin/pawncc": "bin/", "lib/libpawnc.so": "",
	})

	before := snapshotCheckDependency(t, opts.CacheDir)
	opts.Fix = true
	result := checkCompilerReliabilityResult(t, pkg, opts)
	if result.Status != checkPassed || result.CanFix || result.RepairAttempted || result.Err != nil {
		t.Errorf("checkCompilers(verified directory mapping) = %+v, want passed completion verification", result)
	}

	if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
		t.Error("checking a complete directory-mapped compiler changed cached files")
	}
}

func TestCheckCompilerDirectoryMappingDoesNotHideUnknownFiles(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "empty directory"
		}

		t.Run(name, func(t *testing.T) {
			pkg, opts, request, support, _ := installCheckCompilerFixtureWithLayout(t, download.ExtractTgz, "bin/pawncc", map[string]string{
				"bin/pawncc": "bin/", "lib/libpawnc.so": "",
			})
			unknown := filepath.Join(request.Dir, "user-notes")
			if directory {
				if err := os.Mkdir(unknown, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(unknown, []byte("user notes"), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(support, []byte("#define SUPPORT 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			before := snapshotCheckDependency(t, opts.CacheDir)
			opts.Fix = true
			result := checkCompilerReliabilityResult(t, pkg, opts)
			if result.Status != checkFailed || result.CanFix || result.RepairAttempted || result.Err == nil {
				t.Errorf("checkCompilers(directory mapping with unknown %s) = %+v, want failed verification without repair", name, result)
			}

			if got := snapshotCheckDependency(t, opts.CacheDir); !maps.Equal(got, before) {
				t.Errorf("compiler check changed the directory-mapped installation containing an unknown %s", name)
			}
		})
	}
}

func installCheckCompilerFixture(t *testing.T, method string) (pawnpackage.Package, checkOptions, compilerpkg.CompilerFetchRequest, string, string) {
	t.Helper()
	return installCheckCompilerFixtureWithLayout(t, method, "", nil)
}

func installCheckCompilerFixtureWithLayout(t *testing.T, method, binaryName string, paths map[string]string) (pawnpackage.Package, checkOptions, compilerpkg.CompilerFetchRequest, string, string) {
	t.Helper()
	pkg, opts, binary, library := checkCompilerFixture(t, "linux")
	for _, path := range []string{binary, library} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}

	metadata, err := readCheckCompilerMetadata(filepath.Join(opts.CacheDir, "compilers.json"))
	if err != nil {
		t.Fatal(err)
	}

	distribution := metadata[opts.Platform]
	distribution.Method = method
	if binaryName != "" {
		distribution.Binary = binaryName
	}

	if paths != nil {
		distribution.Paths = paths
	}
	metadata[opts.Platform] = distribution
	if err := fs.WriteJSONAtomic(filepath.Join(opts.CacheDir, "compilers.json"), metadata, 0o700, 0o600); err != nil {
		t.Fatal(err)
	}

	request := checkCompilerRequest(pkg.GetBuildConfig("").Compiler.ResolveCompilerConfig(), opts)
	archive := filepath.Join(t.TempDir(), "pawnc."+method)
	writeCheckCompilerArchive(t, archive, method)
	resource := infraresource.NewGitHubReleaseResource(request.Meta, regexp.MustCompile(distribution.Match), infraresource.ResourceTypeCompiler, nil)
	resource.SetCacheDir(opts.CacheDir)
	resource.SetCacheTTL(0)
	resource.SetLocalPath(archive)
	if err := resource.EnsureFromLocal(context.Background(), request.Meta.Tag, ""); err != nil {
		t.Fatal(err)
	}

	hit, cachedArchive := resource.Cached(request.Meta.Tag)
	if !hit {
		t.Fatal("synthetic compiler archive was not cached")
	}

	if _, err := compilerpkg.GetCompilerPackage(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	return pkg, opts, request, filepath.Join(request.Dir, "distribution/include/default.inc"), cachedArchive
}

func writeCheckCompilerArchive(t *testing.T, path, method string) {
	t.Helper()
	files := []struct {
		name, content string
		mode          os.FileMode
	}{
		{name: "bin/pawncc", content: "#!/bin/sh\nexit 0\n", mode: 0o700},
		{name: "lib/libpawnc.so", content: "supporting library", mode: 0o644},
		{name: "bin/pawndisasm", content: "disassembler", mode: 0o755},
		{name: "distribution/include/default.inc", content: checkCompilerSupportContent, mode: 0o644},
	}

	var contents bytes.Buffer
	if method == download.ExtractZip {
		writer := zip.NewWriter(&contents)
		for _, file := range files {
			header := &zip.FileHeader{Name: file.name, Method: zip.Deflate}
			header.SetMode(file.mode)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := entry.Write([]byte(file.content)); err != nil {
				t.Fatal(err)
			}
		}

		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		compressed := gzip.NewWriter(&contents)
		writer := tar.NewWriter(compressed)
		for _, file := range files {
			if err := writer.WriteHeader(&tar.Header{Name: file.name, Mode: int64(file.mode), Size: int64(len(file.content)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}

			if _, err := writer.Write([]byte(file.content)); err != nil {
				t.Fatal(err)
			}
		}

		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		if err := compressed.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(path, contents.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func checkCompilerReliabilityResult(t *testing.T, pkg pawnpackage.Package, opts checkOptions) diagnostic {
	t.Helper()
	results, err := checkCompilers(context.Background(), pkg, opts)
	if err != nil || len(results) != 1 {
		t.Fatalf("checkCompilers() = %+v, %v, want one diagnostic", results, err)
	}

	return results[0]
}

func checkCompilerReliabilityContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != want {
		t.Errorf("ReadFile(%s) = %q, want %q", path, got, want)
	}
}
