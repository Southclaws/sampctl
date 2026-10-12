package commands

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

type checkOutputOptions struct {
	Verbose bool
	Bare    bool
}

type checkOutputStyle struct {
	coloured bool
}

func writeCheckResults(w io.Writer, results []diagnostic, opts checkOutputOptions) (int, error) {
	style := newCheckOutputStyle(w, opts.Bare)
	if file, ok := w.(*os.File); ok && style.coloured {
		w = colorable.NewColorable(file)
	}
	ordered := slices.Clone(results)
	slices.SortStableFunc(ordered, func(a, b diagnostic) int {
		return cmp.Compare(checkStatusPriority(a.Status), checkStatusPriority(b.Status))
	})
	var out bytes.Buffer
	counts := make(map[checkStatus]int)
	var canFix, details bool
	for _, result := range ordered {
		writeCheckResult(&out, result, opts.Verbose, style)
		counts[result.Status]++
		canFix = canFix || result.Status == checkFailed && result.CanFix && !result.RepairAttempted
		details = details || result.Err != nil
	}
	out.WriteString("\n" + style.summary(counts) + "\n")
	if canFix {
		out.WriteString("Run `sampctl check --fix` to attempt supported repairs.\n")
	}
	if details && !opts.Verbose {
		out.WriteString("Run with --verbose for technical details and inspected paths.\n")
	}
	if _, err := w.Write(out.Bytes()); err != nil {
		return 2, fmt.Errorf("write diagnostics: %w", err)
	}
	return boolCount(counts[checkFailed] > 0), nil
}

func newCheckOutputStyle(w io.Writer, bare bool) checkOutputStyle {
	if bare || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return checkOutputStyle{}
	}
	file, ok := w.(*os.File)
	return checkOutputStyle{coloured: ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))}
}

func checkStatusPriority(status checkStatus) int {
	switch status {
	case checkFailed:
		return 0
	case checkWarning:
		return 1
	case checkRepaired:
		return 2
	case checkPassed:
		return 3
	case checkSkipped:
		return 4
	default:
		return 5
	}
}

func writeCheckResult(out *bytes.Buffer, result diagnostic, verbose bool, style checkOutputStyle) {
	out.WriteString("  " + style.resultLine(result) + "\n")
	if result.Action != "" && result.Status != checkPassed && result.Status != checkRepaired {
		out.WriteString("    " + style.text("→", color.FgCyan) + " " + indentCheckDetail(result.Action) + "\n")
	}
	if verbose && result.Path != "" {
		out.WriteString("    Path: " + indentCheckDetail(result.Path) + "\n")
	}
	if verbose && result.Err != nil {
		out.WriteString("    Details: " + indentCheckDetail(result.Err.Error()) + "\n")
	}
}

func indentCheckDetail(text string) string {
	return strings.ReplaceAll(text, "\n", "\n    ")
}

func (s checkOutputStyle) resultLine(result diagnostic) string {
	switch result.Status {
	case checkPassed:
		return s.text("✓", color.FgGreen) + " " + result.Message
	case checkFailed:
		return s.text("✗ "+result.Message, color.FgRed, color.Bold)
	case checkWarning:
		return s.text("! Warning: "+result.Message, color.FgYellow)
	case checkSkipped:
		return s.text("– Skipped: "+result.Message, color.FgHiBlack)
	case checkRepaired:
		return s.text("✓ Repaired:", color.FgCyan) + " " + result.Message
	default:
		return string(result.Status) + ": " + result.Message
	}
}

func (s checkOutputStyle) summary(counts map[checkStatus]int) string {
	var parts []string
	for _, item := range []struct {
		status    checkStatus
		label     string
		plural    string
		attribute color.Attribute
	}{
		{status: checkFailed, label: "issue", plural: "issues", attribute: color.FgRed},
		{status: checkWarning, label: "warning", plural: "warnings", attribute: color.FgYellow},
		{status: checkRepaired, label: "repaired", plural: "repaired", attribute: color.FgCyan},
		{status: checkPassed, label: "passed", plural: "passed", attribute: color.FgGreen},
		{status: checkSkipped, label: "skipped", plural: "skipped", attribute: color.FgHiBlack},
	} {
		count := counts[item.status]
		if count == 0 {
			continue
		}
		label := item.label
		if count != 1 {
			label = item.plural
		}
		parts = append(parts, s.text(fmt.Sprintf("%d %s", count, label), item.attribute))
	}
	if len(parts) == 0 {
		return "No applicable checks."
	}
	return strings.Join(parts, ", ") + "."
}

func (s checkOutputStyle) text(text string, attributes ...color.Attribute) string {
	if !s.coloured {
		return text
	}
	style := color.New(attributes...)
	style.EnableColor()
	return style.Sprint(text)
}
