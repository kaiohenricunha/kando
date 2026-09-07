package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets this test binary re-exec itself as the real kando CLI: when
// KANDO_TEST_MAIN=1 is set, it calls main() directly instead of running the
// test suite. This is the only way to exercise the run* wrappers' exit
// codes and stdout/stderr split, since they call os.Exit — a call that has
// to happen in a real child process, not the test's own. testing.MainStart
// only parses -test.* flags lazily inside m.Run(), which this path never
// reaches, so passing plain kando arguments to the binary is safe.
func TestMain(m *testing.M) {
	if os.Getenv("KANDO_TEST_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// runCLITimeout bounds a single child run. Every verb this harness exercises
// returns promptly; web does not — it blocks in web.Serve until signalled, so
// a runCLI(t, home, "web") would otherwise hang until go test's global
// timeout killed the whole package with no useful message.
const runCLITimeout = 30 * time.Second

// runCLI runs the compiled kando binary (this test binary, re-exec'd via
// TestMain above) against a fresh KANDO_HOME, and captures its exit code and
// both output streams separately — exactly what a script invoking kando
// would see.
//
// The child is addressed by os.Executable rather than os.Args[0]: exec.Command
// runs LookPath on a name with no separator, and argv[0] is whatever the
// caller chose, so a test binary built with `go test -c` and started by bare
// name could re-exec a different binary of that name from $PATH — with
// KANDO_TEST_MAIN set and the assertions grading the wrong process.
func runCLI(t *testing.T, home string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	// An empty home would make kandoRoot fall back to os.UserHomeDir, pointing
	// a mutating verb at the developer's real ~/.kando — and store.Open
	// creates directories and rewrites board.md, so it would write there.
	if home == "" {
		t.Fatal("runCLI needs a non-empty KANDO_HOME")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), runCLITimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "KANDO_TEST_MAIN=1", "KANDO_HOME="+home)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("runCLI %v: did not exit within %s", args, runCLITimeout)
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return outBuf.String(), errBuf.String(), ee.ExitCode()
		}
		t.Fatalf("runCLI %v: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), 0
}

// TestUsageTextIsUnchanged is the golden the "usage() is unchanged for
// existing verbs" claim needs to be a gate rather than an assertion in a PR
// description. Every other check on usage is a substring match that a rewrite
// of the synopsis, the prose or the Environment line would survive.
//
// A deliberate change to the text is a one-line edit here — that is the point:
// it should be deliberate.
func TestUsageTextIsUnchanged(t *testing.T) {
	const want = `usage: kando [board]
       kando web [board] [--port N]
       kando board create <name> | board list [--json]
       kando add <title> [board] [--lane L] [--tag T]
       kando show <card> [board] [--json]
       kando list [board] [--filter "..."] [--json]
       kando move <card> <lane> [board]
       kando tag <card> <tag> [board]
       kando notes <card> [board] (--set TEXT | --file PATH | -)
       kando block <card> --reason "..." [board]
       kando unblock <card> [board]
       kando delete <card> [board]
       kando checklist add <card> <text> [board]
       kando checklist toggle <card> <n> [board] [--was TEXT]
       kando checklist edit <card> <n> <text> [board] [--was TEXT]
       kando archive <card> [board]
       kando archive list [board] [--filter "..."] [--json]
       kando archive restore <card> [board]

Boards live under $KANDO_HOME (default ~/.kando). [board] defaults to "life"
and may come before or after a verb's flags; only "board create",
"kando [board]" and "kando web [board]" create a board — every other verb
needs one that already exists. <card> is a card's id or its exact,
case-insensitive title; a title matching more than one card is refused, and
the error lists the matching ids so a script has an unambiguous way to
retry. <lane> is Backlog, Todo, Doing or Done, case-insensitive. <n> counts
checklist items from 1, as "kando show" lists them; --was TEXT refuses the
change if the item no longer reads that way. --filter takes the same query
syntax as the TUI's / (title text, #tag, !blocked, age>7d, age<3d). kando
archive moves a Done card to the archive; "list", "restore" and the help
words are reserved there, so a card literally titled one of them needs its id.
A board literally named like a verb opens in the terminal with:
kando -- <board>
Environment: KANDO_HOME, KANDO_THEME=paper|ember, NO_COLOR, KANDO_WEB_PORT

Exit codes: 0 ok, 1 the command could not complete (a store, IO or conflict
error), 2 bad arguments (nothing was touched). Stable across every verb, so
a script can rely on them.`
	if usageText != want {
		t.Errorf("usageText changed:\n--- got ---\n%s\n--- want ---\n%s", usageText, want)
	}
}

func TestCLIHelpAndVersion(t *testing.T) {
	home := t.TempDir()

	out, errOut, code := runCLI(t, home, "-h")
	if code != 0 {
		t.Errorf("-h should exit 0, got %d", code)
	}
	if out != "" {
		t.Errorf("-h should print nothing to stdout, got %q", out)
	}
	if !strings.Contains(errOut, "usage: kando") {
		t.Errorf("-h should print usage to stderr, got %q", errOut)
	}

	out, _, code = runCLI(t, home, "-v")
	if code != 0 || strings.TrimSpace(out) != "kando "+version {
		t.Errorf("-v: code=%d out=%q", code, out)
	}
}

func TestCLIMoveArgumentError(t *testing.T) {
	home := t.TempDir()
	out, errOut, code := runCLI(t, home, "move")
	if code != 2 {
		t.Errorf("bad args should exit 2, got %d", code)
	}
	if out != "" {
		t.Errorf("stdout should be empty on an argument error, got %q", out)
	}
	if !strings.HasPrefix(errOut, "kando: ") || !strings.Contains(errOut, "usage: kando") {
		t.Errorf("stderr should carry the error and usage: %q", errOut)
	}
}

func TestCLIMoveNoSuchBoardIsFatal(t *testing.T) {
	home := t.TempDir()
	out, errOut, code := runCLI(t, home, "move", "card", "Doing", "ghost")
	if code != 1 {
		t.Errorf("a store-level error should exit 1, got %d", code)
	}
	if out != "" {
		t.Errorf("stdout should be empty on a fatal error, got %q", out)
	}
	if !strings.HasPrefix(errOut, "kando: ") || strings.Contains(errOut, "usage: kando") {
		t.Errorf("a fatal error must not print usage: %q", errOut)
	}
	if _, err := os.Stat(filepath.Join(home, "ghost")); !os.IsNotExist(err) {
		t.Errorf("a failed move must not create the board directory")
	}
}

// TestCLIMoveMessagesAreUnchanged pins the exact stderr text of the three
// argument and store errors kando move could already produce before the verb
// infrastructure was hoisted out of move.go. Nothing else asserts these: the
// unit tests check only that an error occurred, and the process-level tests
// above check only the "kando: " prefix — which is how two of these drifted in
// the first place. This PR's contract is that kando move behaves identically,
// so the messages are part of it.
func TestCLIMoveMessagesAreUnchanged(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n\n### Renew passport\nid: k7q2m9ab\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			// The remedy has to name a command this binary actually has.
			name: "no such board names a command that works",
			args: []string{"move", "card", "Doing", "ghost"},
			want: `kando: no such board "ghost" (create it first with "kando board create ghost", or open it with "kando ghost")`,
		},
		{
			// Not "needs a card and a lane": the user supplied two arguments,
			// so the useful message names the one that was blank.
			name: "blank card names the card",
			args: []string{"move", "", "Doing"},
			want: "kando: card id or title required",
		},
		{
			// A blank lane falls through to board.ParseLane, which reports the
			// value it could not parse.
			name: "blank lane names the lane",
			args: []string{"move", "k7q2m9ab", "  "},
			want: `kando: invalid lane "  "`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, _ := runCLI(t, home, tc.args...)
			first, _, _ := strings.Cut(errOut, "\n")
			if first != tc.want {
				t.Errorf("stderr first line =\n  %q\nwant\n  %q", first, tc.want)
			}
		})
	}
}

func TestCLIMoveSuccess(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n\n### Renew passport\nid: k7q2m9ab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runCLI(t, home, "move", "k7q2m9ab", "Doing")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if !strings.Contains(out, `moved "Renew passport"`) {
		t.Errorf("stdout = %q", out)
	}
	if errOut != "" {
		t.Errorf("stderr should be empty on success, got %q", errOut)
	}
	data, err := os.ReadFile(filepath.Join(dir, "board.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Doing") || !strings.Contains(string(data), "### Renew passport") {
		t.Errorf("board.md after the move:\n%s", data)
	}
}

func env(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

// OPS-3: default 4242, overridable by --port or KANDO_WEB_PORT, and the
// board name may come before or after the flags.
func TestWebArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantName string
		wantPort int
		wantErr  bool
	}{
		{name: "no arguments", wantPort: defaultPort},
		{name: "board only", args: []string{"work"}, wantName: "work", wantPort: defaultPort},
		{name: "flag before the board", args: []string{"--port", "8080", "work"}, wantName: "work", wantPort: 8080},
		{name: "flag after the board", args: []string{"work", "--port", "8080"}, wantName: "work", wantPort: 8080},
		{name: "single-dash flag", args: []string{"-port", "8080"}, wantPort: 8080},
		{name: "environment", env: map[string]string{"KANDO_WEB_PORT": "9000"}, wantPort: 9000},
		{name: "the flag beats the environment", args: []string{"--port", "8080"}, env: map[string]string{"KANDO_WEB_PORT": "9000"}, wantPort: 8080},
		{name: "unusable environment falls back", env: map[string]string{"KANDO_WEB_PORT": "not-a-port"}, wantPort: defaultPort},
		{name: "out-of-range environment falls back", env: map[string]string{"KANDO_WEB_PORT": "99999"}, wantPort: defaultPort},
		{name: "port zero", args: []string{"--port", "0"}, wantErr: true},
		{name: "port too high", args: []string{"--port", "70000"}, wantErr: true},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
		{name: "two boards", args: []string{"work", "life"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, port, err := webArgs(tc.args, env(tc.env), io.Discard)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got name=%q port=%d", name, port)
				}
				return
			}
			if err != nil {
				t.Fatalf("webArgs: %v", err)
			}
			if name != tc.wantName || port != tc.wantPort {
				t.Errorf("name=%q port=%d, want name=%q port=%d", name, port, tc.wantName, tc.wantPort)
			}
		})
	}
}

func TestEnvPortReportsAnUnusableValue(t *testing.T) {
	var out strings.Builder
	if got := envPort(env(map[string]string{"KANDO_WEB_PORT": "eighty"}), &out, defaultPort); got != defaultPort {
		t.Errorf("port = %d, want %d", got, defaultPort)
	}
	if !strings.Contains(out.String(), "ignoring invalid KANDO_WEB_PORT") {
		t.Errorf("an ignored setting should say so (OPS-4): %q", out.String())
	}
	out.Reset()
	if got := envPort(env(nil), &out, defaultPort); got != defaultPort || out.String() != "" {
		t.Errorf("unset: port=%d out=%q", got, out.String())
	}
}

func TestKandoRootHonoursTheEnvironment(t *testing.T) {
	t.Setenv("KANDO_HOME", "/tmp/kando-test-home")
	if got := kandoRoot(); got != "/tmp/kando-test-home" {
		t.Errorf("kandoRoot = %q", got)
	}
}

// TestCLIJSONContainersAreNeverNull is the test json.go's schema comment asked
// the first --json verb to write, and it is what would have caught
// `archive list --json` emitting "groups": null on an empty archive.
//
// It drives the real binary rather than constructing the structs, because the
// nil slice is introduced in the run* bodies, not in the types — a test that
// built listJSON itself would have passed while the shipped verb was broken.
// An empty board and an untouched archive are the state a fresh board is in,
// so this is the first thing a script hits, not a corner case.
func TestCLIJSONContainersAreNeverNull(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte("## Todo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"list on an empty board", []string{"list", "--json"}},
		{"archive list on an empty archive", []string{"archive", "list", "--json"}},
		{"archive list with a filter matching nothing", []string{"archive", "list", "--json", "--filter", "#nomatch"}},
		{"board list", []string{"board", "list", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, home, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, errOut)
			}
			if strings.Contains(out, "null") {
				t.Errorf("no container may encode as null:\n%s", out)
			}
			var v any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Errorf("output is not valid JSON: %v\n%s", err, out)
			}
		})
	}
}

// TestCLIReadVerbsSucceed drives each read verb through the real binary: the
// four run* wrappers had no process-level coverage at all, which is the layer
// where exit codes, the stdout/stderr split and the --json encoding actually
// live. move has had these since it was written.
func TestCLIReadVerbsSucceed(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	board := "## Todo\n\n### Renew passport\ntag: errand\nid: k7q2m9ab\n"
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte(board), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"board list", []string{"board", "list"}, "life"},
		{"show by id", []string{"show", "k7q2m9ab"}, "Renew passport"},
		{"show by title", []string{"show", "renew passport"}, "k7q2m9ab"},
		{"list", []string{"list"}, "Renew passport"},
		{"list with a filter", []string{"list", "--filter", "#errand"}, `1 of 1 cards match "#errand"`},
		{"archive list on an empty archive", []string{"archive", "list"}, `nothing archived on "life"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, home, tc.args...)
			if code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, errOut)
			}
			if errOut != "" {
				t.Errorf("stderr should be empty on success, got %q", errOut)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("stdout missing %q:\n%s", tc.want, out)
			}
		})
	}

	t.Run("board.md is untouched by every read verb", func(t *testing.T) {
		after, err := os.ReadFile(filepath.Join(dir, "board.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != board {
			t.Errorf("a read verb rewrote board.md:\ngot:\n%s\nwant:\n%s", after, board)
		}
	})
}

// TestCLIReadVerbArgumentErrors pins the exit-code contract for the read
// verbs: an argument-shape error is exit 2 with usage, and stdout stays clean
// so a --json consumer never parses half a message.
func TestCLIReadVerbArgumentErrors(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "life"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"show with no card", []string{"show"}},
		{"show with a blank card", []string{"show", "  "}},
		{"board with no subcommand", []string{"board"}},
		{"archive with no subcommand", []string{"archive"}},
		{"list with too many positionals", []string{"list", "life", "extra"}},
		{"unknown flag", []string{"list", "--nope"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, home, tc.args...)
			if code != 2 {
				t.Errorf("want exit 2, got %d (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout must stay empty on an argument error, got %q", out)
			}
			// Contains, not HasPrefix: a flag-parsing error is printed twice,
			// once by flag.FlagSet (which these verbs point at stderr, as
			// webArgs has always done) and once by usageErr's "kando: " line.
			// Pre-existing and consistent across every flag-taking verb, so it
			// is not this unit's to change — but worth a follow-up, since
			// io.Discard on the FlagSet would leave kando as the only voice.
			if !strings.Contains(errOut, "kando: ") {
				t.Errorf("stderr should carry the kando: error line, got %q", errOut)
			}
		})
	}
}

// TestCLIWriteVerbArgumentErrors is the write-verb twin of
// TestCLIReadVerbArgumentErrors, and it is what would have caught the blank
// card reaching notes, block and unblock: splitBoard stopped rejecting blank
// required values, required() became the compensating check, and three of the
// eight parsers never called it. A per-verb unit test cannot see that class of
// gap, because each one tests the parser that was remembered.
//
// The stakes are not just the exit code. A hand-edited board.md may hold an
// untitled card — markdown.go parses a bare "### " into Title: "" — and
// findCard matches a title case-insensitively, so a blank argument that
// reaches the board silently mutates that card and exits 0.
func TestCLIWriteVerbArgumentErrors(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The untitled card is the trap: without it a blank argument merely fails
	// to match anything, which hides the difference between exit 1 and exit 2.
	seeded := "## Todo\n\n### \nid: bbbbbbbb\n\n### Renew passport\nid: k7q2m9ab\n"
	if err := os.WriteFile(filepath.Join(dir, "board.md"), []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"add with a blank title", []string{"add", "  "}},
		{"delete with a blank card", []string{"delete", ""}},
		{"tag with a blank card", []string{"tag", "", "errand"}},
		{"notes with a blank card", []string{"notes", "", "--set", "x"}},
		{"block with a blank card", []string{"block", "", "--reason", "x"}},
		{"unblock with a blank card", []string{"unblock", ""}},
		{"checklist add with a blank card", []string{"checklist", "add", "", "text"}},
		{"checklist add with blank text", []string{"checklist", "add", "k7q2m9ab", "  "}},
		{"checklist toggle with a blank card", []string{"checklist", "toggle", "", "1"}},
		{"checklist edit with blank text", []string{"checklist", "edit", "k7q2m9ab", "1", ""}},
		{"board create with a blank name", []string{"board", "create", "  "}},
		{"archive with a blank card", []string{"archive", ""}},
		{"archive restore with a blank card", []string{"archive", "restore", "  "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, home, tc.args...)
			if code != 2 {
				t.Errorf("want exit 2, got %d (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout must stay empty on an argument error, got %q", out)
			}
			if !strings.Contains(errOut, "kando: ") {
				t.Errorf("stderr should carry the kando: error line, got %q", errOut)
			}
		})
	}

	t.Run("board.md is untouched by every rejected write", func(t *testing.T) {
		after, err := os.ReadFile(filepath.Join(dir, "board.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != seeded {
			t.Errorf("a rejected write still touched board.md:\ngot:\n%s\nwant:\n%s", after, seeded)
		}
	})
}

// TestCLIWriteVerbsSkipNoOpSaves pins the cardOp contract at the process
// level: a mutation that changes nothing must not rewrite board.md, because
// writeAtomic renames a fresh file into place and store.Version hashes mtime
// and size — so a byte-identical rewrite still bumps the SSE event id and
// wakes every connected kando web client.
func TestCLIWriteVerbsSkipNoOpSaves(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"),
		[]byte("## Todo\n\n### Renew passport\nid: k7q2m9ab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	board := filepath.Join(dir, "board.md")

	// Establish each verb's state once, letting the first call canonicalize
	// the hand-written file, then assert the *second*, identical call is inert.
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"tag with the same tag", []string{"tag", "k7q2m9ab", "errand"}},
		{"block with the same reason", []string{"block", "k7q2m9ab", "--reason", "waiting"}},
		{"notes with the same text", []string{"notes", "k7q2m9ab", "--set", "same"}},
		{"checklist edit to the same text", []string{"checklist", "edit", "k7q2m9ab", "1", "step"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "checklist edit to the same text" {
				if _, errOut, code := runCLI(t, home, "checklist", "add", "k7q2m9ab", "step"); code != 0 {
					t.Fatalf("seeding a checklist item: %s", errOut)
				}
			}
			if _, errOut, code := runCLI(t, home, tc.args...); code != 0 {
				t.Fatalf("first call: exit %d: %s", code, errOut)
			}
			before, err := os.Stat(board)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
			if _, errOut, code := runCLI(t, home, tc.args...); code != 0 {
				t.Fatalf("second call: exit %d: %s", code, errOut)
			}
			after, err := os.Stat(board)
			if err != nil {
				t.Fatal(err)
			}
			if !before.ModTime().Equal(after.ModTime()) {
				t.Errorf("a no-op %s rewrote board.md (mtime %v -> %v); withCard's changed gate should have skipped the save",
					tc.name, before.ModTime(), after.ModTime())
			}
		})
	}
}

// TestCLINotesFileIsBounded pins that --file bounds the read rather than
// reading everything and rejecting afterwards. /dev/zero reports size 0 to
// Stat and never reaches EOF, so os.ReadFile would grow until the process
// died; io.LimitReader stops at the cap and the command exits 1 promptly.
func TestCLINotesFileIsBounded(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero on this platform")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"),
		[]byte("## Todo\n\n### Renew passport\nid: k7q2m9ab\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan int, 1)
	go func() {
		_, _, code := runCLI(t, home, "notes", "k7q2m9ab", "--file", "/dev/zero")
		done <- code
	}()
	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("want exit 1 for an oversized --file, got %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("kando notes --file /dev/zero did not terminate: the read is unbounded")
	}
}

// TestCLIArchiveDispatch pins runArchive's three-way dispatch through the real
// binary. Nothing else does: the unit tests call archiveCard/archiveRestore
// directly, and the argument-error cases assert exit 2 with a "kando: " prefix,
// which is exactly what the pre-PR dispatcher produced for the same inputs.
// Reverting the "restore" arm and the bare-<card> fallthrough left the whole
// suite green before this test existed.
func TestCLIArchiveDispatch(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"),
		[]byte("## Done\n\n### Ship it\nid: k7q2m9ab\ndone: 2026-09-02\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("bare <card> reaches runArchiveCard", func(t *testing.T) {
		out, errOut, code := runCLI(t, home, "archive", "k7q2m9ab")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		if !strings.Contains(out, `archived "Ship it"`) {
			t.Errorf("stdout = %q, want the archived line", out)
		}
	})

	t.Run("list still reaches runArchiveList", func(t *testing.T) {
		out, errOut, code := runCLI(t, home, "archive", "list")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		if !strings.Contains(out, "Ship it") {
			t.Errorf("stdout = %q, want the archived card listed", out)
		}
	})

	t.Run("restore reaches runArchiveRestore", func(t *testing.T) {
		out, errOut, code := runCLI(t, home, "archive", "restore", "k7q2m9ab")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		if !strings.Contains(out, `restored "Ship it" to Doing`) {
			t.Errorf("stdout = %q, want the restored line", out)
		}
	})
}

// TestCLIArchiveReservedWordsNeedAnID is the end-to-end half of the collision
// this dispatch deliberately accepts: "list" and "restore" are matched as
// subcommands before the bare <card> form, so a card literally titled one of
// them is unreachable by title — and reachable by id. That trade is only
// defensible if the id actually works, which is what this asserts through the
// real dispatcher rather than by calling archiveCard directly.
func TestCLIArchiveReservedWordsNeedAnID(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "life")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board.md"),
		[]byte("## Done\n\n### list\nid: aaaaaaaa\ndone: 2026-09-02\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// By title, "list" is the subcommand: it lists an empty archive and
	// leaves the card alone. This is the accepted collision, not a bug.
	out, errOut, code := runCLI(t, home, "archive", "list")
	if code != 0 {
		t.Fatalf("archive list: exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "nothing archived") {
		t.Errorf("archive list should have listed an empty archive, got %q", out)
	}

	// By id, the same card archives normally — the escape the collision relies on.
	out, errOut, code = runCLI(t, home, "archive", "aaaaaaaa")
	if code != 0 {
		t.Fatalf("archive by id: exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `archived "list"`) {
		t.Errorf("stdout = %q, want the card titled \"list\" archived by id", out)
	}
}

// TestCapabilityMatrixHasNoUnbuiltCLICells guards README's "What each surface
// can do" table, which footnote 5 makes a per-PR obligation: every `—` in the
// CLI column becomes a `V` in the pull request that adds its verb, so the
// column says what the CLI can do at that merge point.
//
// That obligation was missed in five consecutive pull requests, because
// nothing enforced it — TestUsageTextIsUnchanged pins the usage text, but the
// table had no equivalent. The CLI column is now complete apart from the three
// deliberate X rows, so the invariant is simply that no `—` remains: a future
// row added as `—` has to be flipped by the unit that ships its verb, or this
// fails.
func TestCapabilityMatrixHasNoUnbuiltCLICells(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	var rows int
	for _, line := range strings.Split(string(data), "\n") {
		// Capability rows only: four columns, and the header/separator skipped.
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "|---") || strings.Contains(line, "| Web | TUI | CLI |") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 4 {
			continue
		}
		rows++
		if cli := strings.TrimSpace(cells[3]); strings.HasPrefix(cli, "—") {
			t.Errorf("capability %q still reads %q in the CLI column; the PR that ships its verb must flip it (README footnote 5)", cells[0], cli)
		}
	}
	if rows < 15 {
		t.Fatalf("only found %d capability rows — the table moved or the parse broke, so this test is not guarding anything", rows)
	}
}
