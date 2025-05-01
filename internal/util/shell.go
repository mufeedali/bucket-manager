// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package util

import "strings"

// QuoteArgForShell wraps an argument for safe use in a remote shell command.
// It handles existing single quotes by closing the quote, adding an escaped quote,
// and reopening. It also handles the tilde prefix correctly for shell expansion.
func QuoteArgForShell(arg string) string {
	// Replace ' with '\''
	cleanedString := strings.ReplaceAll(arg, "'", "'\\''")
	// Handle ~/ prefix separately to allow shell expansion
	if strings.HasPrefix(cleanedString, "~/") {
		// Return tilde outside quotes, quote the rest
		return `~/"` + strings.TrimPrefix(cleanedString, "~/") + `"`
	}
	// Quote the entire argument if no tilde prefix
	return `"` + cleanedString + `"`
}
