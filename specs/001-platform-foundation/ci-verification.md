# CI Verification Record (T086)

**Feature**: `001-platform-foundation` | **Requirement**: FR-035, SC-009

FR-035 requires that any failing automated check causes the verification run to
report failure and identify the failing item. SC-009 requires a 100% interception
rate across three deliberately injected defect classes, and a result within five
minutes of submission.

This file records the outcome of that verification. It is filled in by running
the procedure below against the real pipeline; until then the results table
carries no values, because Constitution Principle III forbids writing an
expected outcome as if it had been measured.

## Procedure

Each defect class is pushed on its own throwaway branch, the run is observed,
and the branch is deleted afterwards.

### 1. Compilation error

```bash
git switch -c ci-verify/compile-error
printf '\nfunc broken() { return undefinedSymbol }\n' >> internal/config/config.go
git commit -am 'ci-verify: inject a compilation error'
git push -u origin ci-verify/compile-error
```

Expect: the **Build and unit test** job fails at `make build`, and the log names
`internal/config/config.go` and the undefined symbol.

### 2. Lint violation

```bash
git switch -c ci-verify/lint-violation
# An unchecked error return: errcheck is enabled in .golangci.yml.
printf '\nfunc leak() { os.Open("/tmp/x") }\n' >> internal/config/parse.go
git commit -am 'ci-verify: inject a lint violation'
git push -u origin ci-verify/lint-violation
```

Expect: the **Lint** job fails at `make lint`, and the log names the file, the
line, and the `errcheck` rule.

### 3. Failing unit test

```bash
git switch -c ci-verify/failing-test
cat >> internal/config/validate_test.go <<'GO'

func TestDeliberateFailure(t *testing.T) {
	t.Fatal("ci-verify: this failure is intentional")
}
GO
git commit -am 'ci-verify: inject a failing test'
git push -u origin ci-verify/failing-test
```

Expect: the **Build and unit test** job fails at `make test`, and the log names
`TestDeliberateFailure`.

### Cleanup

```bash
git switch main
git branch -D ci-verify/compile-error ci-verify/lint-violation ci-verify/failing-test
git push origin --delete ci-verify/compile-error ci-verify/lint-violation ci-verify/failing-test
```

## Results

| # | Defect class | Job expected to fail | Blocked? | Named the failing item? | Run duration | Run URL |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Compilation error | Build and unit test | | | | |
| 2 | Lint violation | Lint | | | | |
| 3 | Failing unit test | Build and unit test | | | | |

**Interception rate**: _not yet measured_
**Feedback time (SC-009 budget: 5 minutes)**: _not yet measured_

## Conditions

Fill in when the runs are performed, so the result is reproducible:

- Date (UTC):
- Commit under test:
- Runner: `ubuntu-latest` (GitHub-hosted)
- Go version: from `go.mod` via `actions/setup-go`
- Workflow file: `.github/workflows/ci.yml` at the commit above
