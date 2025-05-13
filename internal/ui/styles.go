// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle             = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	errorStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	stepStyle              = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	successStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	statusUpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusDownStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusPartialStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusErrorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	statusLoadingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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
