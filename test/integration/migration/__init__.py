"""Migration test package.

Making this directory a package ensures pytest's rootdir/sys.path resolution
doesn't shadow `helpers` against the top-level `test/integration/helpers.py`
(which is the Layer 2 SDK helpers, not the migration helpers).
"""
