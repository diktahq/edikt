#!/usr/bin/env bash
# Negative fixture for db-transactions directive 1
# Property: Transaction handles are passed explicitly rather than via thread-local storage
# In an adopter project this would exercise the violating shape; here it is
# an illustrative stub that always fails (exit 1) to represent violating code.
exit 1
