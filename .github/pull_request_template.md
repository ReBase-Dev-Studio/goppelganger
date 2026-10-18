## Summary

<!-- What does this PR change, and why? One or two sentences. -->

Closes #<!-- issue number -->

## Type of change

<!-- Check all that apply. -->

- [ ] `fix` — bug fix (non-breaking)
- [ ] `feat` — new feature or schema keyword support (non-breaking)
- [ ] `refactor` / `perf` — no behavior change
- [ ] `docs` / `test` / `chore`
- [ ] **Breaking change** — public API change, new error for previously accepted schemas, or changed seeded output

## Changes

<!-- Bullet the notable changes. Point reviewers at the files that matter most. -->

-

## Schema behavior

<!-- Required for parser/generator changes; delete otherwise. -->

**Example schema:**

```json

```

**Output (seed 42) before → after:**

```json

```

## Checklist

### Code

- [ ] No new dependencies — stdlib only
- [ ] Deterministic: no Go map iteration feeding output, no `time.Now`, no global `math/rand`; all randomness flows through the per-instance `*rand.Rand`
- [ ] Contradictory constraints are rejected by the parser (fail fast), not the generator
- [ ] New errors use a sentinel in the package where they originate, are re-exported from the root, and are wrapped with `%w`
- [ ] The walker never mutates the AST (`Schema` stays safe to share)

### Tests

- [ ] Tests added or updated for the changed behavior
- [ ] `go build ./...` and `go vet ./...` pass
- [ ] `go test ./... -race` passes

### Seeded output & docs

- [ ] Seeded output is unchanged — **or** this PR is marked breaking, and the `example_test.go` `// Output:` blocks and the README seed-42 sample are regenerated
- [ ] `README.md` updated (supported keywords, generation rules, errors, limitations) if user-visible behavior changed

## Notes for reviewers

<!-- Trade-offs, follow-ups, open questions, or anything you want a second opinion on. -->
