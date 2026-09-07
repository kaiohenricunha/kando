// Local CI matrix for kando (see /local-attest). Runs on a clean worktree and,
// on a full pass, posts a SHA-pinned attestation comment to the open PR.
export default {
  matrix: [
    // The hard legs call the Makefile targets so there is one declaration of
    // each command. `make test` runs with -race, which needs cgo and a host
    // C toolchain; without one the leg fails to build rather than to test.
    { name: "fmt", mode: "hard", command: "make fmt-check" },
    { name: "vet", mode: "hard", command: "make vet" },
    { name: "test", mode: "hard", command: "make test" },
    // Hard, but only about whether the scan ran — findings themselves still
    // do not block. The pin has to move with the toolchain: govulncheck v1.1.4
    // panics outright on Go 1.27 ("unexpected expr: *ast.KeyValueExpr", out of
    // x/tools' SSA builder), and as a plain advisory leg that crash was
    // indistinguishable from a clean scan, so the repo could believe it was
    // scanning while it was not. This is the only vulnerability gate here —
    // there is no .github/workflows — so that gap mattered.
    //
    // The assertion is on the report, not the exit code: `go run` collapses
    // "vulnerabilities found" (3) and "the scanner failed" (2) into its own
    // exit 1, so the code cannot tell them apart. A summary line can. Either
    // summary passes the leg; a panic or a failed fetch prints neither and
    // fails it.
    {
      name: "vuln",
      mode: "hard",
      command:
        "out=$(go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./... 2>&1); " +
        'printf "%s\\n" "$out"; ' +
        "printf '%s' \"$out\" | grep -qE 'No vulnerabilities found|Your code is affected by'",
    },
  ],
  toolchain: { goMod: "go.mod" },
};
