# Retired Test Suites

Tests in this directory were written against pre-v0.5.0 behavior that no
longer exists. They are kept for archaeology but not run by `test/run.sh`.

## test-install-e2e.sh.retired-v0.5.0

Retired in Phase 5 of v0.5.0 stability release.

Exercised the pre-v0.5.0 flat-payload install.sh against a mock curl,
asserting that a specific set of commands, rules, templates, agents, and
hook scripts landed at specific paths after running install.sh with a
fake $HOME.

After Phase 5, install.sh is a thin launcher bootstrap. The payload
enumeration it used to perform now lives inside the release tarball,
fetched and extracted by `bin/edikt install`. Mock-curl-against-
install.sh is no longer a meaningful test of anything — the v0.5.0
equivalent coverage lives in:

- `test/integration/install/test_fresh_install.sh`
- `test/integration/install/test_v043_cross_major_upgrade.sh`
- `test/integration/install/test_v050_to_v050_noop.sh`
- `test/integration/install/test_install_with_ref_flag.sh`
- `test/integration/install/test_install_dry_run.sh`
- `test/unit/launcher/test_install_*.sh` (launcher payload semantics)
