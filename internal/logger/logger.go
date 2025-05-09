package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var defaultLogger *slog.Logger

// getLogFilePath determines the path for the application log file based on XDG spec.
func getLogFilePath() (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		stateDir = filepath.Join(homeDir, ".local", "state")
	}

	logDir := filepath.Join(stateDir, "bucket-manager")
	logFile := filepath.Join(logDir, "app.log")
	return logFile, nil
}

// setupLogging configures the default logger based on whether to log to file and/or stderr.
func setupLogging(logToFile bool, logToStderr bool) error {
	if !logToFile && !logToStderr {
		logToStderr = true
		fmt.Fprintln(os.Stderr, "Warning: No log output specified, defaulting to stderr.")
	}

	var writers []io.Writer
	var logFileHandle *os.File

	if logToFile {
		logFilePath, err := getLogFilePath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error determining log file path: %v. File logging disabled.\n", err)
		} else {
			logDir := filepath.Dir(logFilePath)
			if err := os.MkdirAll(logDir, 0750); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating log directory %s: %v. File logging disabled.\n", logDir, err)
			} else {
				file, err := os.OpenFile(logFilePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0640)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening log file %s: %v. File logging disabled.\n", logFilePath, err)
				} else {
					writers = append(writers, file)
					logFileHandle = file
				}
			}
		}
	}

	if logToStderr {
		writers = append(writers, os.Stderr)
	}

	var finalWriter io.Writer
	if len(writers) == 0 {
		fmt.Fprintln(os.Stderr, "Error: All log writers failed to initialize. Logging to stderr as fallback.")
		finalWriter = os.Stderr
	} else if len(writers) == 1 {
		finalWriter = writers[0]
	} else {
		finalWriter = io.MultiWriter(writers...)
	}

	// Using JSON handler for structured logging consistency.
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewJSONHandler(finalWriter, opts)
	defaultLogger = slog.New(handler)

	// For a CLI tool, letting the OS close logFileHandle on exit is generally acceptable.
	_ = logFileHandle // Avoid unused variable error if file logging is disabled

	return nil
}

// InitLogger initializes the logger based on the execution mode (TUI or CLI).
// It MUST be called once at the beginning of the application.
func InitLogger(isTUI bool) {
	logToFile := true
	logToStderr := !isTUI

	err := setupLogging(logToFile, logToStderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger initialization failed: %v. Falling back to basic stderr logging.\n", err)
		handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
		defaultLogger = slog.New(handler)
	} else {
		if logToFile {
			logFilePath, pathErr := getLogFilePath()
			if pathErr == nil {
				if logToStderr {
					Info("Logging configured.", "file", logFilePath, "stderr", logToStderr)
				} else {
					fmt.Fprintf(os.Stderr, "Logging to file: %s\n", logFilePath)
				}
			}
		}
	}
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
