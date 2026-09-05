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
    {
      name: "vuln",
      mode: "advisory",
      command: "go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...",
    },
  ],
  toolchain: { goMod: "go.mod" },
};
