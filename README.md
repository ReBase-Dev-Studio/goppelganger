# goppelganger

[![Go Reference](https://pkg.go.dev/badge/github.com/ReBase-Dev-Studio/goppelganger.svg)](https://pkg.go.dev/github.com/ReBase-Dev-Studio/goppelganger)
[![Go Version](https://img.shields.io/badge/go-1.22%2B-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Schema-First Faker for Go. Feed it a JSON Schema, get back random data that satisfies the contract — no structs, no reflection, no third-party faking libraries.

> **Status:** MVP. Stdlib only, Go 1.22+, safe for concurrent use, deterministic with a seed.

## Install

```sh
go get github.com/ReBase-Dev-Studio/goppelganger
```

Requires Go 1.22+.

## Usage

```go
schema := []byte(`{
  "type": "object",
  "required": ["id", "email", "role", "tags", "zip"],
  "properties": {
    "id":    {"type": "string", "format": "uuid"},
    "email": {"type": "string", "format": "email"},
    "role":  {"enum": ["admin", "editor", "viewer"]},
    "age":   {"type": ["integer", "null"], "minimum": 18, "maximum": 99},
    "tags":  {"type": "array", "minItems": 1, "maxItems": 3, "items": {"type": "string", "pattern": "^[a-z]{3,8}$"}},
    "zip":   {"type": "string", "pattern": "^\\d{5}$"}
  }
}`)

// Quick, non-deterministic
user, err := goppelganger.Generate(schema)

// Reproducible (tests)
seed := int64(42)
faker := goppelganger.New(goppelganger.Options{Seed: &seed, Locale: "en_US"})
user, err = faker.Generate(schema)

// Non-object root (e.g. an array of strings)
value, err := faker.GenerateValue([]byte(`{"type": "array", "items": {"type": "string", "format": "ipv4"}}`))

// Compile once, generate many times (skips JSON decoding, parsing and regexp compilation)
userSchema := goppelganger.MustCompile(schema) // or Compile for an error instead of a panic
user, err = faker.GenerateSchema(userSchema)
value, err = goppelganger.GenerateSchemaValue(userSchema)
```

With seed `42`, the first `faker.Generate(schema)` call returns (as JSON):

```json
{
  "email": "winter.banner616@mountain.example.org",
  "id": "c1be0cb7-e57d-4160-b718-3a6e539fecae",
  "role": "admin",
  "tags": ["hfj", "dztbzyna"],
  "zip": "46693"
}
```

- `Generate` requires an object root (directly or via `$ref`), else `ErrRootMustBeObject`; a nullable root may return a `nil` map.
- A `Faker` is safe for concurrent use: each call holds its lock for the whole generation, so a seeded `Faker` yields the same sequence of results for the same sequence of calls.
- `Options.Seed` is a `*int64`: `nil` draws a random seed from `crypto/rand`; any non-nil value (including `0`) is used literally. `Options.Locale` defaults to `DefaultLocale` (`en_US`) when empty.
- `New` never fails: an unsupported locale is returned as `ErrUnsupportedLocale` by every call.
- The package-level `Generate`/`GenerateValue`/`GenerateSchema`/`GenerateSchemaValue` share one randomly seeded `Faker`.
- `Compile` returns an immutable `*Schema`, safe to share across goroutines and Fakers. `Generate(b)` is exactly `Compile(b)` + `GenerateSchema`, so both give the same output for the same seed. `Compile` accepts any root; `GenerateSchema` reports `ErrRootMustBeObject`. A compiled schema is several times faster per call (`go test -bench Generate -benchmem`).
- Results are plain Go values ready for `encoding/json`: `map[string]any`, `[]any`, `string`, `int64` (`integer`), `float64` (`number`), `bool`, `nil`.

Runnable examples (recursive `$ref`, `Compile` reuse, error matching) live in [`example_test.go`](example_test.go) and on [pkg.go.dev](https://pkg.go.dev/github.com/ReBase-Dev-Studio/goppelganger#pkg-examples); `go test` verifies their seeded output.

[`examples/mockserver`](examples/mockserver/main.go) is a stub REST API that serves users generated from an embedded schema ([`schemas/user.json`](examples/mockserver/schemas/user.json)):

```sh
go run ./examples/mockserver -seed 42     # -addr :3000 by default; omit -seed for random data
curl 'localhost:3000/users?count=2'       # 1–50 users, default 3
curl localhost:3000/users/7               # one user with "id": 7
```

## Supported Schema

| Keyword | Notes |
|---------|-------|
| `type` | `object`, `array`, `string`, `integer`, `number`, `boolean`, `null`; `[X, "null"]` and OpenAPI `nullable: true` |
| `enum`, `const` | Typeless schemas allowed when one of these is present |
| `minLength`, `maxLength`, `format`, `pattern` | `format`: `uuid`, `email`, `date-time`, `date`, `time`, `uri`, `ipv4`, `ipv6`, `hostname` (others ignored). `pattern` must be valid RE2 (Go `regexp`), see [Limitations](#limitations) |
| `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum` | draft-04 boolean and draft-06+ numeric forms |
| `multipleOf` | Must be a positive integer for `integer` schemas |
| `properties`, `required` | Property order is preserved; `required` names absent from `properties` are ignored |
| `items`, `minItems`, `maxItems` | Single-schema `items` only (draft-07 tuple form is rejected) |
| `$ref` | Local only (`#`, `#/$defs/...`, `#/definitions/...`, any JSON pointer). Recursive schemas supported; keywords next to `$ref` are ignored |

Unknown keywords are ignored. Unsatisfiable schemas (`minimum > maximum`, `minLength > maxLength`, empty exclusive ranges, `multipleOf` with no multiple in range) are rejected at parse time with `ErrInvalidConstraint`.

## Generation Rules

- `const` wins over `enum`, which wins over everything else.
- Optional properties are included with 50% probability; nullable values are `null` 25% of the time.
- Arrays without `minItems`/`maxItems` get 1–5 items; without `items`, they hold strings.
- Sizes are capped at 10 000 items per array and 10 000 characters per string: a larger `minItems`/`minLength` is rejected at parse time (`ErrSizeLimitExceeded`); a larger `maxItems`/`maxLength` is treated as 10 000.
- Each call's total output is capped at a cost of 1 000 000 (`generator.MaxGenerationCost`): every value costs 1 and every string also costs its length in characters (object keys are free).
  - A schema whose *mandatory* output (`required` properties, `minItems`, `minLength`, `const`/`enum`; `format` strings at their longest possible length, `pattern` strings at their shortest) costs more is rejected with `ErrSizeLimitExceeded` before generating, regardless of seed.
  - Otherwise generation never fails on size: the generator reserves room for what is still mandatory and spends the rest on optional parts. When space runs out, optional properties are omitted, arrays shrink towards `minItems`, plain and `pattern` strings shrink towards their minimum length, `enum` picks among values that fit, and nullable values become `null`.
  - Space is spent in schema order, so in very large outputs later branches end up closer to their minimum than earlier ones. Outputs well below the cap are unaffected.
- Strings are alphanumeric (`[a-zA-Z0-9]`), 5–20 characters unless `minLength`/`maxLength` say otherwise. Booleans are 50/50.
- Known `format`s ignore the 5–20 default; explicit `minLength`/`maxLength` are retried up to 10 times, then `ErrUnsatisfiable`.
  - `uuid` is v4; `date-time`/`date`/`time` are RFC 3339 UTC between 2000-01-01 and 2030-12-31.
  - `email`, `hostname` and `uri` (`https://`) use locale words under RFC 2606 reserved domains (`*.example.com|net|org`).
  - `ipv4` keeps the first octet in 1–223; `ipv6` is 128 random bits.
- `pattern` wins over `format` (in 2020-12 `format` is an annotation, `pattern` an assertion). Values are built from the RE2 syntax tree and must pass `regexp.MatchString`.
  - Generation is length-aware: each value aims at a length drawn uniformly from what the pattern can match, so `^[a-z]{3,8}$` yields every length from 3 to 8 equally often. Default lengths do not apply; explicit `minLength`/`maxLength` narrow the range.
  - Open quantifiers (`*`, `+`, `{n,}`) repeat at most 10 extra times, or more when `minLength` needs it (up to `minLength` + 10), so `^[a-z]+$` + `minLength: 15` yields 15–25 characters.
  - `.` yields alphanumerics; character classes prefer printable ASCII. Patterns are unanchored per JSON Schema, so `abc` yields exactly `abc`.
  - A pattern that cannot match any length within `minLength`/`maxLength` (e.g. `^[a-z]{3}$` + `minLength: 4`) fails with `ErrUnsatisfiable` immediately; otherwise each value gets 10 attempts.
- `integer` values are `int64`, `number` values are `float64`. Unbounded numbers fall in 0–1000; a single bound extends 1000 in the open direction.
- Recursive `$ref`s expand once, then stop: an optional property is omitted, a required one becomes `null`, an array becomes empty (or `minItems` nulls).
- Same `Seed` + same sequence of calls + same library version ⇒ same output. Property order follows the schema; no map iteration, wall clock or global RNG is involved.

## Errors

All errors are wrapped sentinels; match them with `errors.Is`:

| Error | When |
|-------|------|
| `ErrRootMustBeObject` | `Generate` called with a non-object root |
| `ErrUnknownType` / `ErrMissingType` | `type` is unknown or absent |
| `ErrInvalidConstraint` | Contradictory constraints (e.g. `minimum > maximum`) |
| `ErrInvalidPattern` | `pattern` is not valid RE2 |
| `ErrInvalidRef` | `$ref` is remote or cannot be resolved |
| `ErrSizeLimitExceeded` | `minItems` or `minLength` above 10 000, or mandatory output above the 1 000 000 cost cap |
| `ErrUnsatisfiable` | Constraints could not be met after retries |
| `ErrUnsupportedLocale` | Locale other than `en_US` |

Malformed JSON returns the wrapped `encoding/json` error.

## Limitations

- **Not supported (ignored as unknown keywords):** `allOf`/`oneOf`/`anyOf`/`not`, `if`/`then`/`else`, `additionalProperties`, `patternProperties`, `minProperties`/`maxProperties`, `uniqueItems`, `contains`, `prefixItems`.
- **Rejected:** type unions with more than one non-null type (`["string", "integer"]`), draft-07 tuple `items` arrays, remote or unresolvable `$ref`.
- **`pattern`** uses Go's RE2 syntax, not ECMA-262: lookarounds and backreferences are rejected with `ErrInvalidPattern`. See [`regexp/syntax`](https://pkg.go.dev/regexp/syntax) for the accepted grammar.
- **`pattern` lengths are tracked as ranges**, so lengths a pattern skips over (e.g. `^(ab)+$` only matches even lengths) are not known in advance: `^(ab)+$` + `minLength: 3, maxLength: 3` fails with `ErrUnsatisfiable` after 10 attempts, and near the output size cap such a pattern can fail instead of shrinking.
- **`enum`/`const`** values are returned as given and are not checked against `type` or other keywords.
- **Output size cap:** outputs that would grow past the 1 000 000 cost cap (deep nesting, mutually recursive `$ref`s inside arrays) are trimmed, so their optional parts thin out towards the end; schemas that *require* more are rejected.
- **Locales:** only `en_US`. Name-based heuristics (e.g. a `firstName` property producing a first name) are out of scope.

## Development

```sh
go build ./...
go vet ./...
go test ./... -race
go test . -run '^$' -bench Generate -benchmem   # Generate vs precompiled GenerateSchema
```

Seeded output (the sample above and the `// Output:` blocks in `example_test.go`) is pinned per library version: when generation changes, run `go test . -run '^Example'` and paste the new `got:` values.

## Layout

| Path | Responsibility |
|------|----------------|
| `/` | Public API (`goppelganger.go`, `options.go`, `schema.go`); testable examples in `example_test.go` |
| `/parser` | JSON Schema → typed AST |
| `/internal/numeric` | Range math shared by parser and generator |
| `/generator` | AST walker and value generators |
| `/examples/mockserver` | Runnable stub HTTP API built on the library |
| `/.github` | Pull request template |

## Contributing

Issues and pull requests are welcome. Before opening a PR:

- **Stdlib only:** no external dependencies.
- **Determinism:** never iterate Go maps to produce output, never call `time.Now`, never use the global `math/rand`; all randomness flows through the per-instance `*rand.Rand`.
- **Fail fast:** contradictory constraints are rejected by the parser, not the generator. New errors are sentinels wrapped with `%w` and re-exported from the root package.
- **Tests:** add or update tests for the changed behavior; `go build ./...`, `go vet ./...` and `go test ./... -race` must pass.
- **Seeded output:** a change to generated values for a given seed is a breaking change. Mark it as such and regenerate the README sample and the `example_test.go` `// Output:` blocks (see [Development](#development)).
- **Docs:** this README is the user contract; update it when supported keywords, generation rules, errors or limitations change.
- **Commits:** use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`, …).

The [pull request template](.github/pull_request_template.md) walks through this checklist.

## License

[MIT](LICENSE) © 2026 Re:Base Studio
