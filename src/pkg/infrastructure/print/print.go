package print

import (
	"fmt"
	"sync/atomic"

	"github.com/fatih/color"
)

var (
	isVerbose  atomic.Bool
	isColoured atomic.Bool
	infoStyle  = color.New(color.FgBlack).Add(color.BgYellow)
	warnStyle  = color.New(color.FgBlack).Add(color.BgHiRed)
	erroStyle  = color.New(color.FgRed).Add(color.BgBlack)
)

// SetVerbose activates all the Verb calls
func SetVerbose() {
	isVerbose.Store(true)
}

// SetColoured activates ANSI colour codes
func SetColoured() {
	isColoured.Store(true)
}

// Verb prints a message only if Verb is set - controlled via the -v flag
func Verb(a ...interface{}) {
	if isVerbose.Load() {
		Info(a...)
	}
}

// Info is for general purpose messages that are always shown
func Info(a ...interface{}) {
	if isColoured.Load() {
		fmt.Print(infoStyle.Sprint("INFO:"), " ", color.WhiteString(fmt.Sprintln(a...)))
	} else {
		fmt.Print("INFO: ", fmt.Sprintln(a...))
	}
}

// Warn is for warnings that do not prevent the command from finishing
func Warn(a ...interface{}) {
	if isColoured.Load() {
		fmt.Print(warnStyle.Sprint("WARN:"), " ", color.YellowString(fmt.Sprintln(a...)))
	} else {
		fmt.Print("WARN: ", fmt.Sprintln(a...))
	}
}

// Erro is for warnings that do not prevent the command from finishing
func Erro(a ...interface{}) {
	if isColoured.Load() {
		fmt.Print(erroStyle.Sprint("ERROR:"), " ", color.RedString(fmt.Sprintln(a...)))
	} else {
		fmt.Print("ERROR: ", fmt.Sprintln(a...))
	}
}
