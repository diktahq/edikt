# Behavioral fixtures

A directive whose `verify_kind` is `behavioral` claims something can be *demonstrated*, not merely grepped. edikt holds that claim to a pair of executable scripts, and `gov compile` refuses to render until both exist.

If you have hit this error, you are in the right place:

```text
ADR-014: directives[3] "...": verify_kind is behavioral but positive_fixture_path
is empty — a behavioral verify requires a fixture PAIR; see
https://edikt.dev/governance/behavioral-fixtures
```

## Why a pair

One script proves the property **holds** when the code is correct. The other proves the check **fails** when the property is violated. A verification that cannot be shown to fail is not a verification — it is a green light wired to nothing, and the second script is what rules that out.

```yaml
directives:
  - text: "Every hook MUST emit exactly one JSON object. (ref: ADR-061)"
    verify_kind: behavioral
    positive_fixture_path: test/fixtures/behavioral/single-json-object-positive.sh
    negative_fixture_path: test/fixtures/behavioral/single-json-object-negative.sh
```

Each script exits `0` for the outcome its name claims: the positive exits `0` when the property holds, the negative exits `0` when it has successfully detected a violation.

## Where they live, and what they are called

**The compile validates the fixture PATH, not the filename.** Any path that resolves inside your project is accepted. That is deliberate, and it matters more than it looks:

> A one-pair-per-artifact naming scheme breaks the moment one script asserts several directives. A consumer project hit exactly this — one test covering four directives — and no filename convention keyed to a single artifact could express it.

`edikt sidecar approve` defaults to a per-artifact layout when you do not pass `--positive-fixture` / `--negative-fixture`:

```
test/fixtures/behavioral/<artifact-id>/positive.sh
test/fixtures/behavioral/<artifact-id>/negative.sh
```

That default is fine while one artifact means one property. It stops fitting the moment a single script proves a property that several directives assert — the case above — because the path is keyed to the artifact rather than to what is being proven.

When that happens, name the pair after **the property**, and point every directive that asserts it at the same pair:

```
test/fixtures/behavioral/<property-slug>-positive.sh
test/fixtures/behavioral/<property-slug>-negative.sh
```

Nothing in the schema or the compile forbids sharing a pair, and doing so is more honest than duplicating a script to satisfy a naming rule. Pass the paths explicitly with `--positive-fixture` / `--negative-fixture` to override the default.

`test/fixtures/behavioral/` is a convention, not a requirement — put them wherever your project keeps executable test assets, and be consistent.

## Writing the negative arm

The negative arm is the one that gets written carelessly, so be concrete about what it does:

1. Stage a deliberate violation of the property.
2. Run the same check the positive arm runs.
3. Exit `0` **because the check refused**, and non-zero if the check passed against a violation.

A negative fixture that merely exits `0` proves nothing and will pass every gate edikt has. If you cannot make the check fail, the property is not being verified — that is a finding about the check, not about the fixture.

## Before you regenerate a sidecar

A regeneration can introduce behavioral verifies that did not exist before, each one demanding a fixture pair you have not written yet. The next `gov compile` then refuses to render, and the fix you were shipping is blocked by its own sidecar.

`edikt gov compile` warns before dispatching when it is about to do this, naming the count. Take the warning seriously: it is the only point at which the obligation is visible before it blocks you.

To check what you already carry:

```bash
grep -rcE 'verify_kind:[[:space:]]*"?behavioral"?' docs/**/*.edikt.yaml
```

Quote the pattern as shown. The value may be emitted bare or quoted, and a plain `grep 'verify_kind: behavioral'` undercounts.

## Related

- [How Governance Compiles](/governance/compile) — where the fixture gate runs
- [Quality Gates](/governance/gates) — the other checks a compile enforces
