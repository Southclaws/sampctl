package compiler

import (
	"os"
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func TestMain(m *testing.M) {
	print.SetVerbose()
	os.Exit(m.Run())
}
