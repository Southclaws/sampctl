package runtime

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseConfigResourceSetsErrorWhenCloseFails(t *testing.T) {
	t.Parallel()

	var err error
	closeConfigResource(&err, closerFunc(func() error { return errors.New("close failed") }), "wrap message")
	require.EqualError(t, err, "wrap message: close failed")
}

func TestCloseConfigResourceKeepsExistingError(t *testing.T) {
	t.Parallel()

	existing := errors.New("original")
	err := existing
	closeConfigResource(&err, closerFunc(func() error { return errors.New("close failed") }), "wrap message")
	require.ErrorIs(t, err, existing)
}

func TestFromString(t *testing.T) {
	t.Parallel()

	value := "server"
	result, err := fromString("hostname", reflect.ValueOf(&value), false, "")
	require.NoError(t, err)
	assert.Equal(t, "hostname server\n", result)

	result, err = fromString("hostname", reflect.ValueOf((*string)(nil)), false, "fallback")
	require.NoError(t, err)
	assert.Equal(t, "hostname fallback\n", result)

	_, err = fromString("hostname", reflect.ValueOf((*string)(nil)), true, "")
	require.EqualError(t, err, "field hostname is required")
}

func TestFromSlice(t *testing.T) {
	t.Parallel()

	values := []string{"mysql", "streamer"}
	result, err := fromSlice("plugins", reflect.ValueOf(values), false, false)
	require.NoError(t, err)
	assert.Equal(t, "plugins mysql streamer\n", result)

	result, err = fromSlice("gamemode", reflect.ValueOf([]string{"main", "side"}), false, true)
	require.NoError(t, err)
	assert.Equal(t, "gamemode0 main\ngamemode1 side\n", result)

	_, err = fromSlice("plugins", reflect.ValueOf([]string(nil)), true, false)
	require.EqualError(t, err, "field plugins is required")
}

func TestFromBool(t *testing.T) {
	t.Parallel()

	value := true
	result, err := fromBool("announce", reflect.ValueOf(&value), false, "false")
	require.NoError(t, err)
	assert.Equal(t, "announce 1\n", result)

	result, err = fromBool("announce", reflect.ValueOf((*bool)(nil)), false, "false")
	require.NoError(t, err)
	assert.Equal(t, "announce 0\n", result)

	_, err = fromBool("announce", reflect.ValueOf((*bool)(nil)), false, "maybe")
	require.EqualError(t, err, "invalid default bool value \"maybe\" for announce: strconv.ParseBool: parsing \"maybe\": invalid syntax")
}

func TestFromInt(t *testing.T) {
	t.Parallel()

	value := 7777
	result, err := fromInt("port", reflect.ValueOf(&value), false, "0")
	require.NoError(t, err)
	assert.Equal(t, "port 7777\n", result)

	result, err = fromInt("port", reflect.ValueOf((*int)(nil)), false, "7778")
	require.NoError(t, err)
	assert.Equal(t, "port 7778\n", result)

	_, err = fromInt("port", reflect.ValueOf((*int)(nil)), false, "bad")
	require.EqualError(t, err, "invalid default int value \"bad\" for port: strconv.Atoi: parsing \"bad\": invalid syntax")
}

func TestFromFloat(t *testing.T) {
	t.Parallel()

	value := 0.5
	result, err := fromFloat("gravity", reflect.ValueOf(&value), false, "0")
	require.NoError(t, err)
	assert.Equal(t, "gravity 0.500000\n", result)

	result, err = fromFloat("gravity", reflect.ValueOf((*float64)(nil)), false, "1.25")
	require.NoError(t, err)
	assert.Equal(t, "gravity 1.250000\n", result)

	_, err = fromFloat("gravity", reflect.ValueOf((*float64)(nil)), false, "bad")
	require.EqualError(t, err, "invalid default float value \"bad\" for gravity: strconv.ParseFloat: parsing \"bad\": invalid syntax")
}

func TestFromMap(t *testing.T) {
	t.Parallel()

	result, err := fromMap("extra", reflect.ValueOf(map[string]string{"beta": "2", "alpha": "1"}), false)
	require.NoError(t, err)
	assert.Equal(t, "alpha 1\nbeta 2\n", result)

	_, err = fromMap("extra", reflect.ValueOf(map[string]string(nil)), true)
	require.EqualError(t, err, "field extra is required")
}

type closerFunc func() error

func (f closerFunc) Close() error {
	return f()
}

var _ io.Closer = closerFunc(nil)
