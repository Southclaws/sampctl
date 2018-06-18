package analytics

import (
	"bytes"
	"errors"
	"log"
	"testing"
)

// This test ensures that the interface doesn't get changed and stays compatible
// with the *testing.T type.
// If someone were to modify the interface in backward incompatible manner this
// test would break.
func TestTestingLogger(t *testing.T) {
	_ = Logger(t)
}

// This test ensures the standard logger shim to the Logger interface is working
// as expected.
func TestStdLogger(t *testing.T) {
	var buffer bytes.Buffer
	var logger = StdLogger(log.New(&buffer, "test ", 0))

	logger.Logf("Hello World!")
	logger.Logf("The answer is %d", 42)
	logger.Errorf("%s", errors.New("something went wrong!"))

	const ref = `test INFO: Hello World!
test INFO: The answer is 42
test ERROR: something went wrong!
`

	if res := buffer.String(); ref != res {
		t.Errorf("invalid logs from standard logger:\n- expected: %s\n- found: %s", ref, res)
	}
}
