package context

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Snapshot holds everything gathered about the environment and the
// failure, rendered into the LLM prompt by the caller.
type Snapshot struct {
	OS          string
	Arch        string
	CWD         string
	RawLog      string
	CodeSnippet string // lines around the detected error location, if found
	SourceFile  string
	SourceLine  int
	Language    string // best-effort hint, e.g. "Rust" — empty if unrecognized/unknown
}

// filePathLineRe matches "file:line" patterns across virtually any
// language's traceback/compiler-error format:
//   - Python:       File "x.py", line 42
//   - Go/Rust/JS/C/C++/Java/Kotlin/Swift/PHP/Ruby/C#/Scala/Dart/Zig/...:
//     x.<any-extension>:42  or  x.<any-extension>(42) / x.<any-extension>:42:10
// The extension itself is captured but intentionally unconstrained — no
// hardcoded language list — so a new/uncommon language "just works" as
// long as its tool emits a recognizable path:line pattern. Language name
// is inferred from the extension only as a *hint* for the prompt, never
// as a gate on whether the tool can analyze the log at all: even with no
// match, the raw log/traceback is still sent to the LLM for diagnosis.
var filePathLineRe = regexp.MustCompile(
	`(?:File "([^"]+)", line (\d+))` + // Python-style
		`|([\w./\\-]+\.\w+)[:(](\d+)`, // generic path.ext:line or path.ext(line
)

// extToLanguage is a best-effort hint for the prompt, not a gate — an
// unrecognized extension still gets analyzed, just without a named-language
// hint in the prompt.
var extToLanguage = map[string]string{
	"py": "Python", "go": "Go", "js": "JavaScript", "jsx": "JavaScript",
	"ts": "TypeScript", "tsx": "TypeScript", "rb": "Ruby", "java": "Java",
	"kt": "Kotlin", "kts": "Kotlin", "swift": "Swift", "rs": "Rust",
	"c": "C", "h": "C", "cpp": "C++", "cc": "C++", "hpp": "C++",
	"cs": "C#", "php": "PHP", "scala": "Scala", "dart": "Dart",
	"ex": "Elixir", "exs": "Elixir", "erl": "Erlang", "hs": "Haskell",
	"lua": "Lua", "pl": "Perl", "sh": "Shell", "zig": "Zig", "r": "R",
	"jl": "Julia", "clj": "Clojure", "ml": "OCaml", "nim": "Nim",
}

func Capture(rawLog string) Snapshot {
	cwd, _ := os.Getwd()
	snap := Snapshot{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		CWD:    cwd,
		RawLog: rawLog,
	}

	if file, line := findErrorLocation(rawLog); file != "" {
		snap.SourceFile = file
		snap.SourceLine = line
		snap.CodeSnippet = snippetAround(file, line, 5)
		snap.Language = languageForFile(file) // best-effort hint only
	}

	return snap
}

func findErrorLocation(log string) (string, int) {
	matches := filePathLineRe.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return "", 0
	}
	// Take the last match — typically the deepest/most relevant frame
	// in a traceback (closest to where the failure actually happened).
	m := matches[len(matches)-1]
	if m[1] != "" {
		line, _ := strconv.Atoi(m[2])
		return m[1], line
	}
	line, _ := strconv.Atoi(m[4])
	return m[3], line
}

func languageForFile(path string) string {
	ext := strings.ToLower(path)
	if idx := strings.LastIndex(ext, "."); idx != -1 {
		ext = ext[idx+1:]
	} else {
		return ""
	}
	return extToLanguage[ext] // returns "" for unknown extensions, which is fine
}

func snippetAround(path string, line, radius int) string {
	f, err := os.Open(path)
	if err != nil {
		return "" // file not found locally — fine, prompt just lacks a snippet
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	n := 0
	start, end := line-radius, line+radius
	for scanner.Scan() {
		n++
		if n < start || n > end {
			continue
		}
		marker := "  "
		if n == line {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%4d| %s\n", marker, n, scanner.Text())
	}
	return b.String()
}
