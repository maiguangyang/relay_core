/*
 * @Author: Marlon.M
 * @Email: maiguangyang@163.com
 * @Date: 2025-12-24
 */
package utils

import (
	"fmt"
	"sync"
	"time"
)

// LogLevel represents logging level
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogCallback is called when a log message is generated
type LogCallback func(level LogLevel, message string)

// Logger is a simple thread-safe logger with callback support
type Logger struct {
	mu       sync.RWMutex
	level    LogLevel
	callback LogCallback
	prefix   string
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// GetLogger returns the default logger instance
func GetLogger() *Logger {
	once.Do(func() {
		defaultLogger = &Logger{
			level:  LogLevelInfo,
			prefix: "relay",
		}
	})
	return defaultLogger
}

// NewLogger creates a new logger with the given prefix
func NewLogger(prefix string) *Logger {
	return &Logger{
		level:  LogLevelInfo,
		prefix: prefix,
	}
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetCallback sets the log callback
func (l *Logger) SetCallback(callback LogCallback) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callback = callback
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.mu.RLock()
	currentLevel := l.level
	callback := l.callback
	prefix := l.prefix
	l.mu.RUnlock()

	if level < currentLevel {
		return
	}

	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	fullMessage := fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, level.String(), prefix, message)

	if callback != nil {
		callback(level, fullMessage)
	} else {
		fmt.Println(fullMessage)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LogLevelDebug, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LogLevelInfo, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LogLevelWarn, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LogLevelError, format, args...)
}

// Package-level convenience functions

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

// SetLevel sets the log level for the default logger
func SetLevel(level LogLevel) {
	GetLogger().SetLevel(level)
}

// SetCallback sets the callback for the default logger
func SetCallback(callback LogCallback) {
	GetLogger().SetCallback(callback)
}
