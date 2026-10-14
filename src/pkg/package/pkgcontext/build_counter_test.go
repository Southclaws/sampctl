package pkgcontext

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadIntValidatesCounterWithoutChangingFile(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    uint32
		err     error
	}{
		{name: "zero", content: "0"},
		{name: "positive", content: "42", want: 42},
		{name: "negative", content: "-1"},
		{name: "malformed", content: "invalid", err: strconv.ErrSyntax},
		{name: "overflow", content: "9223372036854775808", err: strconv.ErrRange},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "build-number")
			require.NoError(t, os.WriteFile(file, []byte(test.content), 0o600))
			before, err := os.Stat(file)
			require.NoError(t, err)

			number, err := readInt(file)
			switch {
			case test.name == "negative":
				assert.EqualError(t, err, "build number is not a positive integer")
			case test.err != nil:
				assert.ErrorIs(t, err, test.err)
				assert.ErrorContains(t, err, "failed to interpret buildfile contents as an integer number")
			default:
				require.NoError(t, err)
				assert.Equal(t, test.want, number)
			}

			content, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, test.content, string(content))
			after, err := os.Stat(file)
			require.NoError(t, err)
			assert.Equal(t, before.Mode().Perm(), after.Mode().Perm())
		})
	}
}

func TestReadIntInitialisesMissingCounter(t *testing.T) {
	file := filepath.Join(t.TempDir(), "build-number")
	number, err := readInt(file)
	require.NoError(t, err)
	assert.Zero(t, number)
	content, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "0", string(content))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestBuildCounterValidationAndPersistence(t *testing.T) {
	for _, initial := range []string{"-1", "0", "41"} {
		t.Run(initial, func(t *testing.T) {
			pcx := newRunBuildFixture(t)
			file := filepath.Join(pcx.Package.LocalPath, "build-number")
			require.NoError(t, os.WriteFile(file, []byte(initial), 0o600))
			before, err := os.Stat(file)
			require.NoError(t, err)

			_, _, err = pcx.Build(t.Context(), BuildOptions{Name: "release", BuildFile: file})
			want := initial
			if initial == "-1" {
				assert.EqualError(t, err, "build number is not a positive integer")
				assert.NoFileExists(t, filepath.Join(pcx.Package.LocalPath, "build", "release.amx"))
			} else {
				require.NoError(t, err)
				number, parseErr := strconv.Atoi(initial)
				require.NoError(t, parseErr)
				want = strconv.Itoa(number + 1)
			}

			content, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, want, string(content))
			after, err := os.Stat(file)
			require.NoError(t, err)
			assert.Equal(t, before.Mode().Perm(), after.Mode().Perm())
		})
	}
}
