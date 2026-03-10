package runtime

import (
	"os"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func TestMain(m *testing.M) {
	err := os.MkdirAll("./tests/cache", 0o700)
	if err != nil {
		panic(err)
	}

	print.SetVerbose()

	os.Exit(m.Run())
}
