#!/usr/bin/env bash
# Negative fixture for events-bus directive 0
# Property: Published events carry a versioned schema identifier (event_type + schema_version)
# In an adopter project this would exercise the violating shape; here it is
# an illustrative stub that always fails (exit 1) to represent violating code.
exit 1
