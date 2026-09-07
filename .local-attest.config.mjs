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
    // Pinned, and the pin has to move with the toolchain: govulncheck v1.1.4
    // panics outright on Go 1.27 ("unexpected expr: *ast.KeyValueExpr", out of
    // x/tools' SSA builder), so an out-of-date scanner reports a crash rather
    // than a clean scan — which an advisory leg would happily swallow.
    {
      name: "vuln",
      mode: "advisory",
      command: "go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...",
    },
  ],
  toolchain: { goMod: "go.mod" },
};
