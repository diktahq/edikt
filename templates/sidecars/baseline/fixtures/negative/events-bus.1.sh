#!/usr/bin/env bash
# Negative fixture for events-bus directive 1
# Property: Event handlers are idempotent — replaying an event produces no duplicate side effects
# In an adopter project this would exercise the violating shape; here it is
# an illustrative stub that always fails (exit 1) to represent violating code.
exit 1
