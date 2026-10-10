//go:build !edikt_dev

package cmd

// devBuild is false in the build users receive. Files carrying
// //go:build edikt_dev (the gov-benchmark installer) are absent here, so the
// verbs they register do not exist in this binary — not hidden, absent. The
// audience gate asserts it from both sides.
const devBuild = false
