package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type installerFixture struct {
	bin       string
	work      string
	temporary string
	release   string
	calls     string
}

func requireShell(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("installer and documentation scripts require a posix shell")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("posix shell unavailable: %v", err)
	}

	return shell
}

func writeFixtureFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func newInstallerFixture(t *testing.T, extension string) installerFixture {
	t.Helper()

	root := t.TempDir()
	fixture := installerFixture{
		bin:       filepath.Join(root, "bin"),
		work:      filepath.Join(root, "work"),
		temporary: filepath.Join(root, "temporary"),
		release:   filepath.Join(root, "release.json"),
		calls:     filepath.Join(root, "calls"),
	}
	for _, path := range []string{fixture.bin, fixture.work, fixture.temporary} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	var release strings.Builder
	for _, architecture := range []string{"386", "amd64", "arm64", "armv7"} {
		fmt.Fprintf(&release, "  \"browser_download_url\": \"https://example.invalid/sampctl_1.0.0_linux_%s.%s\"\n", architecture, extension)
	}
	writeFixtureFile(t, fixture.release, release.String(), 0o600)
	writeFixtureFile(t, filepath.Join(fixture.bin, "uname"), "#!/bin/sh\nprintf '%s\\n' \"$TEST_MACHINE\"\n", 0o700)
	writeFixtureFile(t, filepath.Join(fixture.bin, "curl"), `#!/bin/sh
url=
output=
while [ "$#" -gt 0 ]; do
	case "$1" in
		-o) shift; output=$1 ;;
		https://*) url=$1 ;;
	esac
	shift
done
case "$url" in
	*/releases/latest)
		printf 'metadata\n' >> "$TEST_CALLS"
		if [ "$TEST_FETCH_FAILURE" = 1 ]; then exit 22; fi
		cat "$TEST_RELEASE"
		;;
	*)
		printf 'download %s\n' "$url" >> "$TEST_CALLS"
		printf 'downloaded payload\n' > "$output"
		if [ "$TEST_DOWNLOAD_FAILURE" = 1 ]; then exit 22; fi
		;;
esac
`, 0o700)
	for _, name := range []string{"tar", "dpkg", "rpm"} {
		writeFixtureFile(t, filepath.Join(fixture.bin, name), fmt.Sprintf("#!/bin/sh\nprintf 'install %s %%s\\n' \"$*\" >> \"$TEST_CALLS\"\n", name), 0o700)
	}
	writeFixtureFile(t, filepath.Join(fixture.bin, "sudo"), "#!/bin/sh\nexec \"$@\"\n", 0o700)

	return fixture
}

func (fixture installerFixture) run(t *testing.T, script, machine string, failures ...string) (string, error) {
	t.Helper()

	script, err := filepath.Abs(script)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), requireShell(t), script)
	command.Dir = fixture.work
	command.Env = append(os.Environ(),
		"PATH="+fixture.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+fixture.temporary,
		"TEST_MACHINE="+machine,
		"TEST_RELEASE="+fixture.release,
		"TEST_CALLS="+fixture.calls,
		"TEST_FETCH_FAILURE=0",
		"TEST_DOWNLOAD_FAILURE=0",
	)
	command.Env = append(command.Env, failures...)
	output, runErr := command.CombinedOutput()

	entries, err := os.ReadDir(fixture.temporary)
	if err != nil || len(entries) != 0 {
		t.Errorf("temporary downloads after script = %v, %v; want none", entries, err)
	}

	return string(output), runErr
}

func (fixture installerFixture) recordedCalls(t *testing.T) string {
	t.Helper()

	contents, err := os.ReadFile(fixture.calls)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}

	return string(contents)
}

func TestInstallersSelectCPUArchitecture(t *testing.T) {
	requireShell(t)
	t.Parallel()

	for _, installer := range []struct {
		script, extension, tool string
	}{
		{"install-bin.sh", "tar.gz", "tar"},
		{"install-deb.sh", "deb", "dpkg"},
		{"install-rpm.sh", "rpm", "rpm"},
	} {
		for _, architecture := range []struct {
			machine, asset string
		}{
			{"x86_64", "amd64"},
			{"amd64", "amd64"},
			{"aarch64", "arm64"},
			{"arm64", "arm64"},
			{"i386", "386"},
			{"i486", "386"},
			{"i586", "386"},
			{"i686", "386"},
			{"armv7", "armv7"},
			{"armv7l", "armv7"},
		} {
			t.Run(installer.script+"/"+architecture.machine, func(t *testing.T) {
				fixture := newInstallerFixture(t, installer.extension)
				if output, err := fixture.run(t, installer.script, architecture.machine); err != nil {
					t.Fatalf("installer failed: %v\n%s", err, output)
				}

				calls := fixture.recordedCalls(t)
				asset := "https://example.invalid/sampctl_1.0.0_linux_" + architecture.asset + "." + installer.extension
				if strings.Count(calls, "download ") != 1 || !strings.Contains(calls, "download "+asset+"\n") {
					t.Errorf("selected download calls = %q; want only %s", calls, asset)
				}
				if strings.Count(calls, "install ") != 1 || !strings.Contains(calls, "install "+installer.tool+" ") {
					t.Errorf("installer calls = %q; want one %s invocation", calls, installer.tool)
				}
			})
		}
	}
}

func TestInstallersStopBeforeInstallingOnFailure(t *testing.T) {
	requireShell(t)
	t.Parallel()

	for _, installer := range []struct {
		script, extension string
	}{
		{"install-bin.sh", "tar.gz"},
		{"install-deb.sh", "deb"},
		{"install-rpm.sh", "rpm"},
	} {
		for _, failure := range []string{"unsupported architecture", "missing asset", "metadata fetch", "archive download"} {
			t.Run(installer.script+"/"+failure, func(t *testing.T) {
				fixture := newInstallerFixture(t, installer.extension)
				machine := "x86_64"
				var failures []string
				switch failure {
				case "unsupported architecture":
					machine = "unsupported-cpu"
				case "missing asset":
					writeFixtureFile(t, fixture.release, "{}\n", 0o600)
				case "metadata fetch":
					failures = []string{"TEST_FETCH_FAILURE=1"}
				case "archive download":
					failures = []string{"TEST_DOWNLOAD_FAILURE=1"}
				}

				if output, err := fixture.run(t, installer.script, machine, failures...); err == nil {
					t.Errorf("installer unexpectedly succeeded after %s: %s", failure, output)
				}
				calls := fixture.recordedCalls(t)
				if strings.Contains(calls, "install ") {
					t.Errorf("installer ran after %s: %s", failure, calls)
				}
				if failure == "unsupported architecture" && calls != "" {
					t.Errorf("unsupported architecture contacted download service: %s", calls)
				}
				if failure != "archive download" && strings.Contains(calls, "download ") {
					t.Errorf("archive download ran after %s: %s", failure, calls)
				}
			})
		}
	}
}

func TestDocgenPreservesReadmeAndPublishesReferenceAtomically(t *testing.T) {
	shell := requireShell(t)
	t.Parallel()

	script, err := os.ReadFile("docgen.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprintf("generator failure=%t", failure), func(t *testing.T) {
			root := t.TempDir()
			for _, directory := range []string{"scripts", "docs", "elsewhere"} {
				if err := os.Mkdir(filepath.Join(root, directory), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			readme := filepath.Join(root, "README.md")
			reference := filepath.Join(root, "docs", "cli-reference.md")
			writeFixtureFile(t, readme, "handwritten introduction\n", 0o600)
			writeFixtureFile(t, reference, "previous reference\n", 0o600)
			writeFixtureFile(t, filepath.Join(root, "scripts", "docgen.sh"), string(script), 0o700)
			generator := "#!/bin/sh\nprintf 'generated reference\\n'\n"
			if failure {
				generator += "exit 1\n"
			}
			writeFixtureFile(t, filepath.Join(root, "sampctl"), generator, 0o700)

			command := exec.CommandContext(t.Context(), shell, filepath.Join(root, "scripts", "docgen.sh"))
			command.Dir = filepath.Join(root, "elsewhere")
			output, runErr := command.CombinedOutput()
			if (runErr != nil) != failure {
				t.Fatalf("docgen failure=%t returned %v: %s", failure, runErr, output)
			}

			contents, err := os.ReadFile(readme)
			if err != nil || string(contents) != "handwritten introduction\n" {
				t.Errorf("readme changed: %q, %v", contents, err)
			}
			wantReference := "generated reference\n"
			if failure {
				wantReference = "previous reference\n"
			}
			contents, err = os.ReadFile(reference)
			if err != nil || string(contents) != wantReference {
				t.Errorf("reference = %q, %v; want %q", contents, err, wantReference)
			}
			entries, err := os.ReadDir(filepath.Dir(reference))
			if err != nil || len(entries) != 1 || entries[0].Name() != "cli-reference.md" {
				t.Errorf("reference directory = %v, %v; want only published reference", entries, err)
			}
		})
	}
}
