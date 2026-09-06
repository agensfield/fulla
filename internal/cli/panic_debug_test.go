//go:build fulla_debug

package cli

import "testing"

func TestPanicProcessPolicy(t *testing.T) { testPanicProcessPolicy(t, true) }
