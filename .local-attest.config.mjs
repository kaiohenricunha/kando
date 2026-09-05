// Local CI matrix for kando (see /local-attest). Runs on a clean worktree and,
// on a full pass, posts a SHA-pinned attestation comment to the open PR.
export default {
  matrix: [
    { name: "fmt", mode: "hard", command: 'test -z "$(gofmt -l .)"' },
    { name: "vet", mode: "hard", command: "go vet ./..." },
    { name: "test", mode: "hard", command: "go test -race -count=1 ./..." },
    {
      name: "vuln",
      mode: "advisory",
      command: "go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...",
    },
  ],
  toolchain: { goMod: "go.mod" },
};
