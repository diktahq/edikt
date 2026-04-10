# Hash Algorithm Reference — PROPOSAL-001 / ADR-008

This file specifies the exact hash computation for `source_hash` and `directives_hash` fields in the directive sentinel block. Any implementation of `/edikt:<artifact>:compile` MUST match these algorithms bit-for-bit, and any test MUST assert against the test vectors in this file.

**Hash function:** SHA-256, hex-encoded, lowercase. 64 characters.

---

## `source_hash` algorithm

### Specification

`source_hash` is the SHA-256 of the artifact body with the directive sentinel block excluded and normalized.

**Steps, in exact order:**

1. Read the full file as UTF-8 text.
2. Remove the entire `[edikt:directives:start]: #` … `[edikt:directives:end]: #` block, inclusive of both sentinel lines. Remove the trailing newline immediately after `[edikt:directives:end]: #` if present.
3. Replace all occurrences of `\r\n` with `\n`.
4. Replace all remaining occurrences of `\r` with `\n`.
5. For each line, remove trailing whitespace (characters in the set `[ \t\v\f]`). Do NOT touch leading whitespace or internal whitespace.
6. Encode the resulting string as UTF-8 bytes.
7. Compute SHA-256 of those bytes.
8. Encode the digest as lowercase hexadecimal.

### Reference implementation (bash + standard POSIX tools)

```bash
source_hash() {
    local file="$1"
    awk '
        /^\[edikt:directives:start\]/ { skip=1; next }
        /^\[edikt:directives:end\]/   { skip=0; next }
        !skip { print }
    ' "$file" | \
    tr -d '\r' | \
    sed 's/[[:space:]]*$//' | \
    shasum -a 256 | \
    awk '{print $1}'
}
```

**Notes on the bash implementation:**
- `awk` removes the directives block. The `next` after `skip=0` prevents the end sentinel line itself from being emitted.
- `tr -d '\r'` handles CRLF line endings by dropping all carriage returns. On Windows-origin files, this normalizes line endings to `\n`-only.
- `sed 's/[[:space:]]*$//'` strips trailing whitespace per line (POSIX character class, portable).
- `shasum -a 256` on macOS; use `sha256sum` on Linux. Same output format.

### Reference implementation (Python 3, authoritative)

When the bash and Python implementations disagree, **the Python one is authoritative**.

```python
import hashlib
import re

def source_hash(file_path: str) -> str:
    """Compute source_hash per ADR-008 specification."""
    with open(file_path, 'r', encoding='utf-8', newline='') as f:
        content = f.read()

    # Remove the directives block (inclusive of sentinels)
    pattern = re.compile(
        r'^\[edikt:directives:start\][^\n]*\n.*?^\[edikt:directives:end\][^\n]*\n?',
        re.MULTILINE | re.DOTALL
    )
    content = pattern.sub('', content)

    # Normalize line endings
    content = content.replace('\r\n', '\n').replace('\r', '\n')

    # Strip trailing whitespace per line
    lines = content.split('\n')
    lines = [line.rstrip(' \t\v\f') for line in lines]
    content = '\n'.join(lines)

    # SHA-256 hex digest
    return hashlib.sha256(content.encode('utf-8')).hexdigest()
```

---

## `directives_hash` algorithm

### Specification

`directives_hash` is the SHA-256 of the canonicalized `directives:` list items (auto list only).

**Steps, in exact order:**

1. Parse the directive block's YAML content between the sentinels.
2. Extract the `directives:` list. If the key is absent, treat as empty list.
3. Do NOT read `manual_directives:`. Do NOT read `suppressed_directives:`.
4. For each item in the list, preserve its exact string content as stored in YAML.
5. Join all items with a single `\n` (LF only, no `\r\n`), in document order.
6. Encode the resulting string as UTF-8 bytes.
7. Compute SHA-256.
8. Encode as lowercase hex.

### Reference implementation (Python 3, authoritative)

```python
import hashlib
import yaml
import re

def directives_hash(file_path: str) -> str:
    """Compute directives_hash per ADR-008 specification."""
    with open(file_path, 'r', encoding='utf-8') as f:
        content = f.read()

    # Extract the directives block content between sentinels
    match = re.search(
        r'^\[edikt:directives:start\][^\n]*\n(.*?)^\[edikt:directives:end\]',
        content,
        re.MULTILINE | re.DOTALL,
    )
    if not match:
        # No block = no directives = hash of empty string
        return hashlib.sha256(b'').hexdigest()

    block_yaml = match.group(1)
    parsed = yaml.safe_load(block_yaml) or {}
    directives_list = parsed.get('directives', []) or []

    # Canonicalize: join with LF
    canonical = '\n'.join(str(item) for item in directives_list)

    return hashlib.sha256(canonical.encode('utf-8')).hexdigest()
```

### Empty list handling

An empty `directives:` list hashes the empty string:

```
SHA-256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

A list with one item `"Foo"` hashes just the string `"Foo"`:

```
SHA-256("Foo") = 1cbec737f863e4922cee63cc2ebbfaafcd1cff8b790d8cfd2e6a5d550b648afa
```

A list with two items `["Foo", "Bar"]` hashes `"Foo\nBar"`:

```
SHA-256("Foo\nBar") = 45a16b4ed72162d1f2b0a7aa0497a2b1bbc1e3dddb4acf7167f7c8a0a7db52ec
```

---

## Test vectors

All implementations MUST produce these exact outputs for these exact inputs. These are the regression tests for any hash implementation.

### Test vector 1: Empty body, empty directives

**Input file (`test1.md`):**

```markdown
```

(completely empty)

**Expected:**
```
source_hash     = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
directives_hash = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

(both are SHA-256 of the empty string)

### Test vector 2: Simple body, no directive block

**Input file (`test2.md`):**

```markdown
# ADR-001: Use PostgreSQL

## Decision
We use PostgreSQL for relational storage.
```

(note: terminated by a single `\n` after the last line)

**Expected:**
```
source_hash     = (compute via reference implementation)
directives_hash = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

(directives_hash is empty because there is no block, so no list to hash)

### Test vector 3: Body with directive block, one auto directive

**Input file (`test3.md`):**

```markdown
# ADR-003: Transaction Boundaries

## Decision
Use transactions for multi-table writes.

[edikt:directives:start]: #
source_hash: xxx
directives_hash: xxx
compiler_version: "0.3.0"
paths:
  - "**/*.go"
scope:
  - implementation
directives:
  - "Always use transactions for multi-table writes (ref: ADR-003)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
```

**Expected:**
```
source_hash     = (compute via reference implementation, EXCLUDES the block)
directives_hash = SHA-256("Always use transactions for multi-table writes (ref: ADR-003)")
                = (compute via reference implementation)
```

Note: `source_hash` is computed over the body AFTER the block is stripped:

```
# ADR-003: Transaction Boundaries

## Decision
Use transactions for multi-table writes.

```

(trailing newlines preserved modulo normalization)

### Test vector 4: Hand-edited directives (user added a line)

**Input file (`test4.md`):** same as test3.md but `directives:` now has two items:

```yaml
directives:
  - "Always use transactions for multi-table writes (ref: ADR-003)"
  - "User added this by hand"
```

**Expected:**
```
source_hash     = <same as test3.md> (body unchanged)
directives_hash = SHA-256("Always use transactions for multi-table writes (ref: ADR-003)\nUser added this by hand")
```

**Behavior:** `source_hash` matches the stored value but `directives_hash` does NOT. Compile enters the hand-edit interview path.

### Test vector 5: Body changed, directives unchanged

**Input file (`test5.md`):** test3.md body modified (e.g., a section reworded), directives list unchanged.

**Expected:**
```
source_hash     = <different from test3.md>
directives_hash = <same as test3.md>
```

**Behavior:** `source_hash` mismatches, compile enters the slow path: regenerates `directives:` from the new body, updates both hashes.

### Test vector 6: manual_directives/suppressed_directives don't affect directives_hash

**Input file (`test6.md`):** test3.md with manual_directives and suppressed_directives populated:

```yaml
directives:
  - "Always use transactions for multi-table writes (ref: ADR-003)"
manual_directives:
  - "User added enforcement"
suppressed_directives:
  - "Previously generated rule we rejected"
```

**Expected:**
```
directives_hash = <same as test3.md>  (only the auto `directives:` list is hashed)
```

**Behavior:** populating `manual_directives:` or `suppressed_directives:` does NOT change `directives_hash`. Compile's fast path still matches on unchanged body.

### Test vector 7: CRLF normalization

**Input file (`test7.md`):** same body as test2.md but with CRLF line endings throughout.

**Expected:**
```
source_hash = <same as test2.md>
```

**Behavior:** CRLF is normalized to LF before hashing, so Windows-origin files produce identical hashes to Unix-origin files with the same content.

### Test vector 8: Trailing whitespace normalization

**Input file (`test8.md`):** same body as test2.md but with trailing spaces added to some lines.

**Expected:**
```
source_hash = <same as test2.md>
```

**Behavior:** trailing whitespace is stripped before hashing. Editors that add/remove trailing whitespace don't cause spurious regeneration.

### Test vector 9: Leading whitespace preserved

**Input file (`test9.md`):** same body as test2.md but with a code block containing indented lines.

```markdown
# ADR-009

## Decision

    def foo():
        return 1
```

**Expected:**
```
source_hash != source_hash of test2.md
```

**Behavior:** leading whitespace is NOT stripped. Indented code blocks produce distinct hashes from unindented ones. Meaningful whitespace is preserved.

---

## Implementation verification

To verify an implementation matches this spec:

1. Run the Python reference implementation on test vector fixture files.
2. Run the bash reference implementation on the same fixtures.
3. Compare outputs. They MUST match.
4. Run your implementation on the same fixtures. Output MUST match both reference implementations.

**Any implementation diverging from these outputs is broken and MUST be fixed, not accommodated.**

---

## Where test fixtures live

Test fixture files matching the test vectors above are at:

`docs/architecture/proposals/PROPOSAL-001-spec/fixtures/hashes/test-vector-{1..9}.md`

Each fixture has a companion `expected.txt` file with the expected hash outputs.

---

## Common mistakes to avoid

1. **Computing `source_hash` over the whole file.** The directive block MUST be excluded. Otherwise, writing new directives immediately invalidates the hash.
2. **Forgetting to normalize line endings.** Files edited on Windows will hash differently from files edited on macOS/Linux if you skip this step.
3. **Stripping leading whitespace.** Don't — meaningful indentation (code blocks, YAML inside prose) must be preserved.
4. **Using JSON serialization for `directives_hash`.** JSON adds quotes, escaping, commas. Use the documented `\n`-join canonicalization.
5. **Including `manual_directives:` or `suppressed_directives:` in `directives_hash`.** Only the auto `directives:` list is hashed.
6. **Using uppercase hex.** The hash field MUST be lowercase hex. Some tools (e.g., PowerShell) output uppercase by default.
7. **Using base64 or another encoding.** Lowercase hex, 64 characters.
