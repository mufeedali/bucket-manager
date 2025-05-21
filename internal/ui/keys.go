// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// This file defines the keyboard bindings for the TUI application.
// It maps keys to actions and provides descriptions for the help menu.

package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the application.
// These bindings are used throughout the TUI for navigation and actions.
type KeyMap struct {
	// Navigation keys
	Up     key.Binding // Move cursor up
	Down   key.Binding // Move cursor down
	Left   key.Binding // Move cursor left/previous screen
	Right  key.Binding // Move cursor right/next screen
	PgUp   key.Binding // Page up in lists
	PgDown key.Binding // Page down in lists
	Home   key.Binding // Jump to top of list
	End    key.Binding // Jump to bottom of list

	// General UI control
	Quit     key.Binding // Exit the application
	Enter    key.Binding // Confirm selection
	Esc      key.Binding // Cancel/go back
	Back     key.Binding // Go back to previous view
	Select   key.Binding // Select an item
	Tab      key.Binding // Next field in forms
	ShiftTab key.Binding // Previous field in forms
	Yes      key.Binding // Confirm in prompts
	No       key.Binding // Deny in prompts

	// Stack management actions
	Config        key.Binding // Access configuration menu
	UpAction      key.Binding // Start/up the selected stack(s)
	DownAction    key.Binding // Stop/down the selected stack(s)
	RefreshAction key.Binding // Restart the selected stack(s)
	PullAction    key.Binding // Pull images for the selected stack(s)

	// Host/SSH configuration actions
	Remove key.Binding // Remove an item (SSH host)
	Add    key.Binding // Add a new item (SSH host)
	Import key.Binding // Import from SSH config
	Edit   key.Binding // Edit an item (SSH host)

	// Misc actions
	ToggleDisabled key.Binding // Toggle disabled state for a host
	PruneAction    key.Binding // Prune containers/images
}

// DefaultKeyMap provides the default keybindings.
var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	PgUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	),
	PgDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home"),
		key.WithHelp("home", "home"),
	),
	End: key.NewBinding(
		key.WithKeys("end"),
		key.WithHelp("end", "end"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select/confirm"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back/cancel"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "b"),
		key.WithHelp("esc/b", "back"),
	),
	Select: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle select"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Yes: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yes"),
	),
	No: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "no"),
	),

	Config: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "configure hosts"),
	),
	UpAction: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "up stack(s)"),
	),
	DownAction: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "down stack(s)"),
	),
	RefreshAction: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh stack(s)"),
	),
	PullAction: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pull images"),
	),

	Remove: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "remove host"),
	),
	Add: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add host"),
	),
	Import: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "import from file"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit host"),
	),

	ToggleDisabled: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle disabled"),
	),
	PruneAction: key.NewBinding(
		key.WithKeys("P"),
		key.WithHelp("P", "prune host"),
	),
}
