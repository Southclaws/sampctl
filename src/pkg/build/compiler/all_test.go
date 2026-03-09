package compiler

import (
	"testing"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func TestMain(m *testing.M) {
	print.SetVerbose()
	m.Run()
}
