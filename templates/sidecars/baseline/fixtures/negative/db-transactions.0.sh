#!/usr/bin/env bash
# Negative fixture for db-transactions directive 0
# Property: Multi-table writes are wrapped in a transaction that rolls back on error
# In an adopter project this would exercise the violating shape; here it is
# an illustrative stub that always fails (exit 1) to represent violating code.
exit 1
