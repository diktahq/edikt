#!/usr/bin/env bash
# Pins ADR-017 / HI-9, LOW-7 — default permissions block in settings.json.tmpl.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── 1. Template is valid JSON after placeholder substitution.
python3 -c "
import json, sys
t = open('templates/settings.json.tmpl').read().replace('\${EDIKT_HOOK_DIR}', '/tmp/h')
data = json.loads(t)
if 'permissions' not in data:
    print('[ADR-017] templates/settings.json.tmpl missing permissions block', file=sys.stderr); sys.exit(1)
perms = data['permissions']
for k in ('deny', 'allow', 'defaultMode'):
    if k not in perms:
        print(f'[ADR-017] permissions.{k} missing', file=sys.stderr); sys.exit(1)
# Required deny entries.
required_deny_substrings = [
    'rm -rf /**',
    'sudo',
    'WebFetch(http://**)',
    '.ssh/id_',
    '.aws/credentials',
    'git push --force main',
]
for needle in required_deny_substrings:
    if not any(needle in d for d in perms['deny']):
        print(f'[ADR-017] deny list missing required pattern: {needle!r}', file=sys.stderr); sys.exit(1)
# Required allow entries.
required_allow_substrings = ['git :', 'pytest', './test/run.sh', 'WebFetch(https://**)']
for needle in required_allow_substrings:
    if not any(needle in a for a in perms['allow']):
        print(f'[ADR-017] allow list missing required pattern: {needle!r}', file=sys.stderr); sys.exit(1)
if perms['defaultMode'] != 'askBeforeAllow':
    print(f'[ADR-017] defaultMode must be askBeforeAllow, got {perms[\"defaultMode\"]!r}', file=sys.stderr); sys.exit(1)
" || fail=1

# ── 2. LOW-7: PostToolUse if-clause excludes node_modules and .venv.
if ! grep -q '!Write(\*\*/node_modules/\*\*)' templates/settings.json.tmpl; then
    echo "[LOW-7] node_modules not excluded from PostToolUse formatter" >&2
    fail=1
fi

exit $fail
