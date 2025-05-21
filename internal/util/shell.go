// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package util provides general utility functions used across the bucket manager application.
// It includes helper functions for string manipulation, shell commands, and other common tasks.
package util

import "strings"

// QuoteArgForShell quotes an argument for safe use in a POSIX shell command.
// It uses single quotes and escapes any internal single quotes following the POSIX shell escaping rules.
// Special handling for "~/" prefix allows shell tilde expansion (relies on remote shell behavior).
//
// This is particularly important for safely executing commands on remote hosts via SSH,
// where arguments might contain spaces or special shell characters.
func QuoteArgForShell(arg string) string {
	// Handle ~/ prefix separately to allow shell expansion. This relies on the
	// remote shell correctly expanding ~ even when the rest is quoted.
	if strings.HasPrefix(arg, "~/") {
		// Quote the part after ~/
		quotedPart := strings.ReplaceAll(arg[2:], "'", `'\''`)
		return `~/'` + quotedPart + `'`
	}

	// For other arguments, replace internal ' with '\'' and wrap in single quotes.
	quotedArg := strings.ReplaceAll(arg, "'", `'\''`)
	return `'` + quotedArg + `'`
}
