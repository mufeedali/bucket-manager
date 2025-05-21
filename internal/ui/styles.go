// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package ui's styles.go file defines the visual styling for the TUI application.
// It uses the lipgloss library to create consistent text and UI element styles
// with appropriate colors, borders, and formatting.

package ui

import "github.com/charmbracelet/lipgloss"

var (
	// General UI element styles
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")) // Purple title text
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // Red error messages
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))            // Blue status messages
	stepStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // Yellow step indicators
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))            // Green success messages
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))             // Magenta cursor indicator

	// Stack status indicator styles
	statusUpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // Green for "up" status
	statusDownStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // Red for "down" status
	statusPartialStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))  // Yellow for "partial" status
	statusErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange for "error" status
	statusLoadingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))   // Grey for "loading" status
	serverNameStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Italic(true)
	identifierColor        = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	mainContentBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder(), true).
				BorderForeground(lipgloss.Color("238")) // Light grey border

	// Footer / Status Bar Styles
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")) // Default light grey text

	footerKeyStyle = lipgloss.NewStyle().
			Inherit(footerStyle).
			Foreground(lipgloss.Color("39")) // Bright blue for key

	footerDescStyle = lipgloss.NewStyle().
			Inherit(footerStyle).
			Foreground(lipgloss.Color("250")) // Light grey for description

	footerSeparatorStyle = lipgloss.NewStyle().
				Inherit(footerStyle).
				Foreground(lipgloss.Color("240")) // Dim grey for separator "|"
)
