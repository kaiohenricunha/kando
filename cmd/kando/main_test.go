package main

import (
	"io"
	"strings"
	"testing"
)

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
