// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package util

import "strings"

func QuoteArgForShell(arg string) string {
	// Replace ' with '\''
	cleanedString := strings.ReplaceAll(arg, "'", "'\\''")
	// Handle ~/ prefix separately to allow shell expansion
	if strings.HasPrefix(cleanedString, "~/") {
		// Return tilde outside quotes, quote the rest
		return `~/"` + strings.TrimPrefix(cleanedString, "~/") + `"`
	}
	return `"` + cleanedString + `"`
}
