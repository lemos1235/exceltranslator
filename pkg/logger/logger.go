package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// LogLevel defines the severity of a log message.
type LogLevel int

const (
	ERROR LogLevel = iota
	WARN
	INFO
	DEBUG
	TRACE
)

// Logger is a custom logger that stores messages in memory and prints to stdout.
type Logger struct {
	mu          sync.Mutex
	logMessages []string    // In-memory buffer for logs to be displayed on frontend
	stdLogger   *log.Logger // Standard library logger for stdout
	maxLines    int         // Max number of lines to store
	minLevel    LogLevel    // Minimum level to output/store
	disabled    bool        // Whether logging is disabled
}

// NewLogger creates a new Logger instance.
func NewLogger(maxLines int) *Logger {
	return &Logger{
		stdLogger:   log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile),
		maxLines:    maxLines,
		logMessages: make([]string, 0, maxLines),
		minLevel:    DEBUG, // Default to DEBUG
	}
}

// SetLevel updates the minimum log level.
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// SetDisabled sets whether logging is disabled.
func (l *Logger) SetDisabled(disabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.disabled = disabled
}

// GetLevel returns the current minimum log level.
func (l *Logger) GetLevel() LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.minLevel
}

// logf formats according to a format specifier and writes to the logger.
func (l *Logger) logf(level LogLevel, format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.disabled || levelRank(level) < levelRank(l.minLevel) {
		return
	}

	msg := fmt.Sprintf(format, v...)
	logEntry := fmt.Sprintf("[%s] %s", strings.ToUpper(level.String()), msg)

	// Output to stdout/stderr (depending on log.Logger setup)
	l.stdLogger.Output(2, logEntry) // Use Output to get correct file/line number

	l.logMessages = append(l.logMessages, logEntry)
	if len(l.logMessages) > l.maxLines {
		// Truncate from the beginning, keep only the last 'maxLines' entries
		l.logMessages = l.logMessages[len(l.logMessages)-l.maxLines:]
	}
}

// Infof logs an info message.
func (l *Logger) Infof(format string, v ...interface{}) {
	l.logf(INFO, format, v...)
}

// Warnf logs a warning message.
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logf(WARN, format, v...)
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logf(ERROR, format, v...)
}

// Debugf logs a debug message.
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logf(DEBUG, format, v...)
}

// Tracef logs a trace message.
func (l *Logger) Tracef(format string, v ...interface{}) {
	l.logf(TRACE, format, v...)
}

// GetLogs returns the current logs from the buffer as a string slice.
func (l *Logger) GetLogs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Return a copy to prevent external modification
	logs := make([]string, len(l.logMessages))
	copy(logs, l.logMessages)
	return logs
}

// Clear removes all in-memory log messages.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logMessages = l.logMessages[:0]
}

func (l LogLevel) String() string {
	switch l {
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case DEBUG:
		return "DEBUG"
	case TRACE:
		return "TRACE"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string representation of a log level to its LogLevel equivalent.
func ParseLevel(level string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "TRACE":
		return TRACE
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO // Default to INFO
	}
}

func levelRank(level LogLevel) int {
	switch level {
	case TRACE:
		return 0
	case DEBUG:
		return 1
	case INFO:
		return 2
	case WARN:
		return 3
	case ERROR:
		return 4
	default:
		return 5
	}
}
