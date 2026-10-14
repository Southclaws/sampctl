package compiler

import (
	"bufio"
	stderrors "errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Southclaws/sampctl/src/pkg/build"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

//nolint:lll
var (
	// matches warnings or errors
	matchCompilerProblem = regexp.MustCompile(`^(.*?)\(([0-9]*)[- 0-9]*\) \: (fatal error|error|user warning|warning)\s?([0-9]*)\: (.*)$`)

	// Header size:             60 bytes
	matchHeader = regexp.MustCompile(`^Header size:\s*([0-9]+) bytes$`)

	// Code size:              276 bytes
	matchCode = regexp.MustCompile(`^Code size:\s*([0-9]+) bytes$`)

	// Data size:                0 bytes
	matchData = regexp.MustCompile(`^Data size:\s*([0-9]+) bytes$`)

	// Stack/heap size:      16384 bytes; estimated max. usage=8 cells (32 bytes)
	matchStack = regexp.MustCompile(`^Stack/heap size:\s*([0-9]*) bytes; estimated max. usage=[0-9]+ cells \(([0-9]+) bytes\)$`)

	// Total requirements:   16720 bytes
	matchTotal = regexp.MustCompile(`^Total requirements:\s*([0-9]+) bytes$`)
)

type compilerOutputParser struct {
	reader     io.Reader
	workingDir string
	errorDir   string
	relative   bool
	done       chan struct{}
	problems   build.Problems
	result     build.Result
	err        error
	output     strings.Builder
}

func newCompilerOutputParser(reader io.Reader, workingDir, errorDir string, relative bool) *compilerOutputParser {
	return &compilerOutputParser{
		reader:     reader,
		workingDir: workingDir,
		errorDir:   errorDir,
		relative:   relative,
		done:       make(chan struct{}),
	}
}

func (p *compilerOutputParser) Run() {
	scanner := bufio.NewScanner(p.reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if p.output.Len() < 64*1024 {
			p.output.WriteString(scanner.Text())
			p.output.WriteByte('\n')
		}
		p.handleLine(scanner.Text())
	}

	if p.err = scanner.Err(); p.err != nil {
		// Drain the pipe so a compiler with invalid output can still exit.
		_, drainErr := io.Copy(io.Discard, p.reader)
		p.err = stderrors.Join(p.err, drainErr)
	}

	close(p.done)
}

func (p *compilerOutputParser) Wait() (build.Problems, build.Result) {
	<-p.done
	return p.problems, p.result
}

func (p *compilerOutputParser) handleLine(line string) {
	groups := matchCompilerProblem.FindStringSubmatch(line)
	if len(groups) == 6 {
		p.handleProblem(groups)
		return
	}

	p.handleResultLine(line)
}

func (p *compilerOutputParser) handleProblem(groups []string) {
	problem := build.Problem{}
	if filepath.IsAbs(groups[1]) {
		problem.File = groups[1]
	} else {
		problem.File = filepath.Join(p.workingDir, groups[1])
	}

	if string(filepath.Separator) != `\\` {
		problem.File = strings.ReplaceAll(problem.File, "\\", "/")
	}
	problem.File = filepath.Clean(problem.File)
	if p.relative {
		if rel, err := filepath.Rel(p.errorDir, problem.File); err == nil {
			problem.File = rel
		}
	}

	lineNumber, err := strconv.Atoi(groups[2])
	if err != nil {
		return
	}
	problem.Line = lineNumber

	switch groups[3] {
	case "user warning":
		fallthrough
	case "warning":
		problem.Severity = build.ProblemWarning
	case "error":
		problem.Severity = build.ProblemError
	case "fatal error":
		problem.Severity = build.ProblemFatal
	}

	problem.Code = groups[4]
	problem.Description = groups[5]
	fmt.Println(problem.String())
	p.problems = append(p.problems, problem)
}

func (p *compilerOutputParser) handleResultLine(line string) {
	switch {
	case strings.HasPrefix(line, "Pawn compiler"):
		return
	case strings.HasPrefix(line, "Compilation aborted"):
		return
	case strings.HasSuffix(line, "Error."):
		return
	case len(strings.TrimSpace(line)) == 0:
		return
	}

	if g := matchHeader.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Header = value
		}
		return
	}

	if g := matchCode.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Code = value
		}
		return
	}

	if g := matchData.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Data = value
		}
		return
	}

	if g := matchStack.FindStringSubmatch(line); len(g) == 3 {
		stackHeap, err := strconv.Atoi(g[1])
		if err != nil {
			return
		}

		estimate, err := strconv.Atoi(g[2])
		if err != nil {
			return
		}

		p.result.StackHeap = stackHeap
		p.result.Estimate = estimate
		return
	}

	if g := matchTotal.FindStringSubmatch(line); len(g) == 2 {
		if value, err := strconv.Atoi(g[1]); err == nil {
			p.result.Total = value
		}
		return
	}

	// preserve unknown result lines for potential debugging
	print.Verb("compiler output:", line)
}
