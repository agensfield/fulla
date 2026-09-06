//go:build !fulla_debug

package cli

// Default builds, including go install, never print recovered panic values or stacks.
const debugPanics = false
