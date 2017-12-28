package print

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	isVerbose = false
	infoStyle = color.New(color.FgBlack).Add(color.BgYellow)
	warnStyle = color.New(color.FgBlack).Add(color.BgHiRed)
	erroStyle = color.New(color.FgRed).Add(color.BgBlack)
)

// SetVerbose activates all the Verb calls
func SetVerbose() {
	isVerbose = true
}

// Verb prints a message only if Verb is set - controlled via the -v flag
func Verb(a ...interface{}) {
	if isVerbose {
		Info(a...)
	}
}

// Info is for general purpose messages that are always shown
func Info(a ...interface{}) {
	fmt.Print(infoStyle.Sprint("INFO:"), " ", color.WhiteString(fmt.Sprintln(a...)))
}

// Warn is for warnings that do not prevent the command from finishing
func Warn(a ...interface{}) {
	fmt.Print(warnStyle.Sprint("WARN:"), " ", color.YellowString(fmt.Sprintln(a...)))
}

// Erro is for warnings that do not prevent the command from finishing
func Erro(a ...interface{}) {
	fmt.Print(erroStyle.Sprint("ERROR:"), " ", color.RedString(fmt.Sprintln(a...)))
}
