//go:build !windows

package main

import "io"

// enableVT does nothing: terminals outside Windows take escape sequences as they are.
func enableVT(io.Writer) {}
