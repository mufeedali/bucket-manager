// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings for the application.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PgUp     key.Binding
	PgDown   key.Binding
	Home     key.Binding
	End      key.Binding
	Quit     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Back     key.Binding
	Select   key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	Yes      key.Binding
	No       key.Binding

	Config        key.Binding
	UpAction      key.Binding
	DownAction    key.Binding
	RefreshAction key.Binding
	PullAction    key.Binding

	Remove key.Binding
	Add    key.Binding
	Import key.Binding
	Edit   key.Binding

	ToggleDisabled key.Binding
	PruneAction    key.Binding
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
