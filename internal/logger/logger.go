package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// TODO: Allow configuration of log level (e.g., via env var or config file)
// TODO: Consider log rotation

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
		// Default to stderr if neither is specified, to ensure logs aren't lost.
		logToStderr = true
		fmt.Fprintln(os.Stderr, "Warning: No log output specified, defaulting to stderr.")
	}

	var writers []io.Writer
	var logFileHandle *os.File // Keep track of the file handle if opened

	if logToFile {
		logFilePath, err := getLogFilePath()
		if err != nil {
			// Log error to stderr since file logging failed.
			fmt.Fprintf(os.Stderr, "Error determining log file path: %v. File logging disabled.\n", err)
			// Continue without file logging if path fails
		} else {
			logDir := filepath.Dir(logFilePath)
			// Create directory with appropriate permissions (0750: user rwx, group rx, others ---)
			if err := os.MkdirAll(logDir, 0750); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating log directory %s: %v. File logging disabled.\n", logDir, err)
			} else {
				// Open file for appending (0640: user rw, group r, others ---)
				file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening log file %s: %v. File logging disabled.\n", logFilePath, err)
				} else {
					writers = append(writers, file)
					logFileHandle = file // Store handle for potential future closure (though usually handled by OS on exit)
				}
			}
		}
	}

	if logToStderr {
		writers = append(writers, os.Stderr)
	}

	var finalWriter io.Writer
	if len(writers) == 0 {
		// Fallback if all writers failed to initialize (should be rare)
		fmt.Fprintln(os.Stderr, "Error: All log writers failed to initialize. Logging to stderr as fallback.")
		finalWriter = os.Stderr
	} else if len(writers) == 1 {
		finalWriter = writers[0]
	} else {
		finalWriter = io.MultiWriter(writers...)
	}

	// Configure the default logger instance
	// Using JSON handler for structured logging consistency.
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo, // Default level
		// AddSource: true, // Uncomment to add source file/line to logs
	}
	handler := slog.NewJSONHandler(finalWriter, opts)
	defaultLogger = slog.New(handler)

	// Note: logFileHandle is not explicitly closed here.
	// For a long-running application, managing file handle closure might be needed.
	// For a CLI tool, letting the OS close it on exit is generally acceptable.
	_ = logFileHandle // Avoid unused variable error if file logging is disabled

	return nil // Indicate success
}

// InitLogger initializes the logger based on the execution mode (TUI or CLI).
// It MUST be called once at the beginning of the application.
func InitLogger(isTUI bool) {
	logToFile := true     // Always attempt to log to file
	logToStderr := !isTUI // Log to stderr only if NOT TUI

	err := setupLogging(logToFile, logToStderr)
	if err != nil {
		// If setup fails, fallback to basic stderr logging to ensure errors are visible
		fmt.Fprintf(os.Stderr, "Logger initialization failed: %v. Falling back to basic stderr logging.\n", err)
		handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
		defaultLogger = slog.New(handler)
	} else {
		// Log the path being used if file logging was successfully set up
		if logToFile {
			logFilePath, pathErr := getLogFilePath()
			if pathErr == nil {
				// Use the logger itself to report where it's logging (if stderr is enabled)
				// Or print directly to stderr otherwise
				if logToStderr {
					Info("Logging configured.", "file", logFilePath, "stderr", logToStderr)
				} else {
					// If only logging to file, print a startup message to stderr so user knows where logs are
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
		// Prevent panic if SetLogger is called before InitLogger
		fmt.Fprintln(os.Stderr, "Warning: SetLogger called before InitLogger. Initializing with defaults.")
		InitLogger(false) // Initialize with CLI defaults
	}
	defaultLogger = l
}

// checkLogger ensures the logger is initialized before use, preventing nil panics.
func checkLogger() {
	if defaultLogger == nil {
		fmt.Fprintln(os.Stderr, "Error: Logger accessed before InitLogger was called. Initializing with defaults.")
		InitLogger(false) // Initialize with CLI defaults as a safety measure
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
// Note: slog prefers structured logging over formatted strings.
// This function is kept for compatibility but using Error with key-value pairs is recommended.
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
// Note: slog prefers structured logging over formatted strings.
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
// Note: slog prefers structured logging over formatted strings.
func Warnf(format string, v ...interface{}) {
	checkLogger()
	defaultLogger.Warn(fmt.Sprintf(format, v...))
}
