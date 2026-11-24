//go:build !windows

package unitycli

import "context"

// runElevated is Windows-only: elsewhere the CLI asks for the sudo password
// itself, given a terminal.
var runElevated func(ctx context.Context, path string, args []string) (processResult, error)
