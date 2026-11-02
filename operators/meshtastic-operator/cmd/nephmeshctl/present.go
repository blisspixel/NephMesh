/*
Copyright 2026 The NephMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/debug"
	"strings"
)

const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
	ansiDim    = "\x1b[2m"
	ansiReset  = "\x1b[0m"
)

const rootUsage = `usage: nephmeshctl <command> [flags]

commands:
  plan        dry-run a CommunicationIntent (no cluster)
  spectrum    per-band occupancy from an SDR sweep CSV
  advise      ask a local model for a report-only action
  decode      Meshtastic headers from decoded LoRa payloads
  resilience  delivery before and after a perturbation
  version     print the version

examples:
  nephmeshctl plan -f examples/regional-intent.yaml
  nephmeshctl spectrum -f examples/spectrum-sweep-sample.csv
run 'nephmeshctl <command> -h' for flags
`

// isTerminal reports whether w is a character device (a terminal), not a pipe
// or an in-memory buffer.
func isTerminal(w any) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// allowColor is the NO_COLOR / TERM=dumb rule. Colour is off unless the output
// is a terminal. An empty NO_COLOR does not disable colour; a non-empty one does.
func allowColor(terminal bool) bool {
	if !terminal || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

// unicodeOK is false for TERM=dumb and for a locale that is not UTF-8. An empty
// locale (common on Windows) keeps the block characters.
func unicodeOK() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	loc := os.Getenv("LC_ALL")
	if loc == "" {
		loc = os.Getenv("LC_CTYPE")
	}
	if loc == "" {
		loc = os.Getenv("LANG")
	}
	if loc == "" {
		return true
	}
	u := strings.ToUpper(loc)
	return strings.Contains(u, "UTF-8") || strings.Contains(u, "UTF8")
}

func chooseFormat(explicit string, terminal bool) (string, error) {
	if explicit == "" {
		if terminal {
			return "text", nil
		}
		return "json", nil
	}
	if explicit != "json" && explicit != "text" {
		return "", fmt.Errorf("invalid -o %q", explicit)
	}
	return explicit, nil
}

func versionString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "nephmeshctl unknown"
	}
	ver := info.Main.Version
	if ver == "" {
		ver = "unknown"
	}
	rev := ""
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		}
	}
	if rev == "" {
		return "nephmeshctl " + ver
	}
	return "nephmeshctl " + ver + " " + rev
}

func commandHelp(w io.Writer, usageLine, summary string, flags []string) {
	fprintln(w, "usage: nephmeshctl "+usageLine)
	fprintln(w, "  "+summary)
	if len(flags) == 0 {
		return
	}
	fprintln(w, "")
	fprintln(w, "flags:")
	for _, fl := range flags {
		fprintln(w, "  "+fl)
	}
}

func cliError(stdout, stderr io.Writer, format, cmd, problem, next string, raw error) int {
	line := problem
	if next != "" {
		line += ". " + next
	}
	if allowColor(isTerminal(stderr)) {
		line = decorateError(problem, next)
	}
	if cmd == "" {
		fprintf(stderr, "nephmeshctl: %s\n", line)
	} else {
		fprintf(stderr, "nephmeshctl %s: %s\n", cmd, line)
	}
	if format == "json" && raw != nil {
		enc := json.NewEncoder(stdout)
		_ = enc.Encode(map[string]string{"error": raw.Error()})
	}
	return exitUsage
}

func resolveInputPath(fs *flag.FlagSet, file string, stdinTerminal bool) (string, error) {
	rest := fs.Args()
	switch len(rest) {
	case 0:
		if file == "-" && stdinTerminal {
			return "", errors.New("stdin is a terminal, so there is nothing to read")
		}
		return file, nil
	case 1:
		if file != "-" {
			return "", errors.New("pass either -f or one file path, not both")
		}
		return rest[0], nil
	default:
		return "", errors.New("unexpected arguments")
	}
}

func inputNextStep(err error) string {
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), "not both"):
		return "Drop one of them."
	case strings.Contains(err.Error(), "unexpected"):
		return "Pass one file, or -f."
	default:
		return "Pass -f with a file path."
	}
}

// inputError is a read failure whose human sentence stays free of the Go chain.
// Unwrap keeps that chain for -o json.
type inputError struct {
	problem string
	raw     error
}

func (e *inputError) Error() string { return e.problem }
func (e *inputError) Unwrap() error { return e.raw }

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, &inputError{problem: "cannot read stdin", raw: err}
		}
		return b, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		problem := fmt.Sprintf("cannot read %s", path)
		if os.IsNotExist(err) {
			problem = fmt.Sprintf("cannot read %s (file not found)", path)
		}
		return nil, &inputError{problem: problem, raw: err}
	}
	return b, nil
}

func failRead(stdout, stderr io.Writer, format, cmd string, err error) int {
	problem := err.Error()
	raw := err
	var ie *inputError
	if errors.As(err, &ie) {
		problem = ie.problem
		raw = ie.raw
	}
	return cliError(stdout, stderr, format, cmd, problem, "Pass -f with a path that exists.", raw)
}

// colorizeReport paints the verdict word on each plain-text line. It does not
// touch JSON. Escape sequences are added only when on is true.
func colorizeReport(text string, on bool) string {
	if !on || text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = colorizeLine(line)
	}
	return strings.Join(lines, "\n")
}

func colorizeLine(line string) string {
	switch {
	case strings.HasPrefix(line, "feasible:"):
		return paint("feasible", ansiGreen) + line[len("feasible"):]
	case strings.HasPrefix(line, "infeasible:"):
		// The word is the verdict. The compiler message after it is a reason detail.
		return paint("infeasible", ansiRed) + paint(line[len("infeasible"):], ansiDim)
	case strings.HasPrefix(line, "airtime: within"):
		return "airtime: " + paint("within", ansiGreen) + line[len("airtime: within"):]
	case strings.HasPrefix(line, "airtime: over"):
		return "airtime: " + paint("over", ansiYellow) + line[len("airtime: over"):]
	case strings.HasPrefix(line, "airtime: not evaluated"):
		return "airtime: " + paint("not evaluated", ansiDim)
	case strings.HasPrefix(line, "recommendation:"):
		return colorizeAction(line)
	case strings.Contains(line, "verdict:"):
		return colorizeVerdict(line)
	default:
		return line
	}
}

func colorizeAction(line string) string {
	rest := strings.TrimPrefix(line, "recommendation: ")
	action := rest
	if i := strings.IndexAny(rest, " -"); i > 0 {
		action = rest[:i]
	}
	code := ansiDim
	switch action {
	case "change_preset", "investigate":
		code = ansiYellow
	case "hold":
		code = ansiDim
	}
	return "recommendation: " + paint(action, code) + rest[len(action):]
}

func colorizeVerdict(line string) string {
	idx := strings.Index(line, "verdict:")
	if idx < 0 {
		return line
	}
	head := line[:idx+len("verdict:")]
	tail := line[idx+len("verdict:"):]
	// One outcome word. "recovered" wins over "fell" when a sentence contains both.
	switch {
	case strings.Contains(tail, "not healthy"):
		return head + paintPhrase(tail, "not healthy", ansiYellow)
	case strings.Contains(tail, "recovered"):
		return head + paintPhrase(tail, "recovered", ansiGreen)
	case strings.Contains(tail, "kept delivering"):
		return head + paintPhrase(tail, "kept delivering", ansiGreen)
	case strings.Contains(tail, "held across"):
		return head + paintPhrase(tail, "held across", ansiGreen)
	case strings.Contains(tail, "fell"):
		return head + paintPhrase(tail, "fell", ansiRed)
	default:
		return line
	}
}

func paintPhrase(s, phrase, code string) string {
	i := strings.Index(s, phrase)
	if i < 0 {
		return s
	}
	return s[:i] + paint(phrase, code) + s[i+len(phrase):]
}

// decorateError paints the problem red and any single-quoted command in the
// next step cyan. JSON output never calls this.
func decorateError(problem, next string) string {
	line := paint(problem, ansiRed)
	if next == "" {
		return line
	}
	return line + ". " + paintQuoted(next)
}

func paintQuoted(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		i := strings.IndexByte(s, '\'')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i+1:], '\'')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		j += i + 1
		b.WriteString(s[:i])
		b.WriteByte('\'')
		b.WriteString(paint(s[i+1:j], ansiCyan))
		b.WriteByte('\'')
		s = s[j+1:]
	}
	return b.String()
}

func paint(s, code string) string {
	return code + s + ansiReset
}

func isUnreachable(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connectex") ||
		strings.Contains(s, "actively refused") ||
		strings.Contains(s, "no such host")
}
