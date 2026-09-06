package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// runCLI runs the compiled kando binary (this test binary, re-exec'd via
// TestMain above) against a fresh KANDO_HOME, and captures its exit code and
// both output streams separately — exactly what a script invoking kando
// would see.
func runCLI(t *testing.T, home string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "KANDO_TEST_MAIN=1", "KANDO_HOME="+home)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), ee.ExitCode()
		}
		t.Fatalf("runCLI %v: %v", args, err)
	}
	return outBuf.String(), errBuf.String(), 0
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
