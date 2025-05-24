// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2025 Mufeed Ali

// Package logger provides structured logging capabilities for the bucket manager application.
// It handles log configuration, redirection to appropriate outputs, and provides
// wrapper functions for different log levels.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent // No logging to stderr, file only
)

// InterfaceType represents the type of interface using the logger
type InterfaceType int

const (
	InterfaceCLI InterfaceType = iota
	InterfaceTUI
	InterfaceWeb
)

// Config holds logger configuration
type Config struct {
	Interface  InterfaceType
	Level      LogLevel
	EnableFile bool
	Verbose    bool // For CLI: enables human-readable stderr logging
	Silent     bool // For CLI: disables all stderr output
}

// defaultLogger is the package-level logger instance used by all logging functions
var defaultLogger *slog.Logger

// getLogFilePath determines the path for the application log file based on XDG spec and interface type.
func getLogFilePath(interfaceType InterfaceType) (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		stateDir = filepath.Join(homeDir, ".local", "state")
	}

	logDir := filepath.Join(stateDir, "bucket-manager")

	// Create interface-specific log file names
	var logFileName string
	switch interfaceType {
	case InterfaceCLI:
		logFileName = "cli.log"
	case InterfaceTUI:
		logFileName = "tui.log"
	case InterfaceWeb:
		logFileName = "web.log"
	default:
		logFileName = "app.log" // fallback
	}

	logFile := filepath.Join(logDir, logFileName)
	return logFile, nil
}

// toSlogLevel converts our LogLevel to slog.Level
func toSlogLevel(level LogLevel) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// createFileWriter creates a file writer for logging, returns nil if file logging should be disabled
func createFileWriter(interfaceType InterfaceType) (io.Writer, error) {
	logFilePath, err := getLogFilePath(interfaceType)
	if err != nil {
		return nil, fmt.Errorf("error determining log file path: %w", err)
	}

	logDir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return nil, fmt.Errorf("error creating log directory %s: %w", logDir, err)
	}

	file, err := os.OpenFile(logFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("error opening log file %s: %w", logFilePath, err)
	}

	return file, nil
}

// setupLogging configures the logger based on the provided configuration
func setupLogging(config Config) error {
	var stderrHandler, fileHandler slog.Handler

	// Always try to set up file logging unless explicitly disabled
	if config.EnableFile {
		fileWriter, err := createFileWriter(config.Interface)
		if err != nil {
			// Only warn about file logging failure in verbose CLI mode
			if config.Interface == InterfaceCLI && config.Verbose {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		} else {
			// JSON format for file logging (good for debugging)
			fileHandler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
				Level: toSlogLevel(LevelDebug), // Always debug level for file
			})
		}
	}

	// Configure stderr logging based on interface type
	switch config.Interface {
	case InterfaceCLI:
		if config.Verbose && !config.Silent {
			// Human-readable format for CLI stderr when verbose
			stderrHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: toSlogLevel(config.Level),
			})
		}
		// If not verbose or silent, no stderr logging for CLI

	case InterfaceTUI:
		// TUI never logs to stderr to avoid polluting the interface

	case InterfaceWeb:
		// Web interface logs to stderr for server monitoring
		stderrHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: toSlogLevel(config.Level),
		})
	}

	// Create the final logger
	if stderrHandler != nil && fileHandler != nil {
		// Both stderr and file - need a custom handler that routes to both
		defaultLogger = slog.New(&multiHandler{
			stderrHandler: stderrHandler,
			fileHandler:   fileHandler,
		})
	} else if stderrHandler != nil {
		defaultLogger = slog.New(stderrHandler)
	} else if fileHandler != nil {
		defaultLogger = slog.New(fileHandler)
	} else {
		// Fallback - should rarely happen
		defaultLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return nil
}

// multiHandler is a custom handler that routes logs to multiple handlers
type multiHandler struct {
	stderrHandler slog.Handler
	fileHandler   slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.stderrHandler.Enabled(ctx, level) || h.fileHandler.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var err1, err2 error
	if h.stderrHandler.Enabled(ctx, record.Level) {
		err1 = h.stderrHandler.Handle(ctx, record)
	}
	if h.fileHandler.Enabled(ctx, record.Level) {
		err2 = h.fileHandler.Handle(ctx, record)
	}

	// Return the first error if any
	if err1 != nil {
		return err1
	}
	return err2
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		stderrHandler: h.stderrHandler.WithAttrs(attrs),
		fileHandler:   h.fileHandler.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		stderrHandler: h.stderrHandler.WithGroup(name),
		fileHandler:   h.fileHandler.WithGroup(name),
	}
}

// InitLogger initializes the logger based on the execution mode (TUI or CLI).
// It MUST be called once at the beginning of the application.
// Deprecated: Use InitCLI, InitTUI, or InitWeb instead for better control.
func InitLogger(isTUI bool) {
	if isTUI {
		InitTUI()
	} else {
		InitCLI(false, false) // Default: not verbose, not silent
	}
}

// InitCLI initializes the logger for CLI interface
func InitCLI(verbose, silent bool) {
	config := Config{
		Interface:  InterfaceCLI,
		Level:      LevelInfo,
		EnableFile: true,
		Verbose:    verbose,
		Silent:     silent,
	}

	if err := setupLogging(config); err != nil {
		// Fallback logging
		defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	// Log initialization with context
	Info("CLI interface started", "verbose", verbose, "silent", silent)
}

// InitTUI initializes the logger for TUI interface (file only, no stderr)
func InitTUI() {
	config := Config{
		Interface:  InterfaceTUI,
		Level:      LevelInfo,
		EnableFile: true,
		Verbose:    false,
		Silent:     false, // Not relevant for TUI
	}

	if err := setupLogging(config); err != nil {
		// Fallback to discard logger for TUI to avoid polluting interface
		defaultLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	// Log initialization with context
	Info("TUI interface started")
}

// InitWeb initializes the logger for Web interface
func InitWeb(level LogLevel) {
	config := Config{
		Interface:  InterfaceWeb,
		Level:      level,
		EnableFile: true,
		Verbose:    false,
		Silent:     false,
	}

	if err := setupLogging(config); err != nil {
		// Fallback logging for web interface
		defaultLogger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	// Log initialization with context
	Info("Web interface started", "level", level)
}

// GetLogFilePath returns the log file path for a given interface type
func GetLogFilePath(interfaceType InterfaceType) (string, error) {
	return getLogFilePath(interfaceType)
}

// EnableVerbose can be called to enable verbose CLI logging after initialization
func EnableVerbose() {
	InitCLI(true, false)
}

// EnableSilent can be called to enable silent CLI logging after initialization
func EnableSilent() {
	InitCLI(false, true)
}

// SetLogger allows replacing the default logger instance.
// This could be used for testing or allowing different parts of the application
// to use specialized loggers. Should be used *after* InitLogger if needed.
func SetLogger(l *slog.Logger) {
	if defaultLogger == nil {
		fmt.Fprintln(os.Stderr, "Warning: SetLogger called before InitLogger. Initializing with defaults.")
		InitLogger(false)
	}
	defaultLogger = l
}

// checkLogger ensures the logger is initialized before use, preventing nil panics.
func checkLogger() {
	if defaultLogger == nil {
		fmt.Fprintln(os.Stderr, "Error: Logger accessed before InitLogger was called. Initializing with defaults.")
		InitLogger(false)
	}
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	checkLogger()
	defaultLogger.Info(msg, args...)
}

// Infof logs a formatted informational message.
// Note: slog prefers structured logging over formatted strings.
// This function is kept for compatibility but using Info with key-value pairs is recommended.
func Infof(format string, v ...interface{}) {
	checkLogger()
	defaultLogger.Info(fmt.Sprintf(format, v...))
}

// Error logs an error message.
func Error(msg string, args ...any) {
	checkLogger()
	defaultLogger.Error(msg, args...)
}

// Errorf logs a formatted error message.
func Errorf(format string, v ...interface{}) {
	checkLogger()
	defaultLogger.Error(fmt.Sprintf(format, v...))
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	checkLogger()
	defaultLogger.Debug(msg, args...)
}

// Debugf logs a formatted debug message.
func Debugf(format string, v ...interface{}) {
	checkLogger()
	defaultLogger.Debug(fmt.Sprintf(format, v...))
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	checkLogger()
	defaultLogger.Warn(msg, args...)
}

// Warnf logs a formatted warning message.
func Warnf(format string, v ...interface{}) {
	checkLogger()
	defaultLogger.Warn(fmt.Sprintf(format, v...))
}
