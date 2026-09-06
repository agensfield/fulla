//go:build fulla_debug

package cli

// This explicit development build opts into Go's panic diagnostics. It must not
// be used with real secrets: panic values and stacks can contain private data.
const debugPanics = true
