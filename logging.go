package logging

import (
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
)

// Configuration constants that define the behavior of the logging system.
// These values can be adjusted based on system requirements.
const (
    DefaultBufferSize = 1000    // Number of messages that can be buffered before blocking
    LogDirName       = "logs"   // Name of the directory where log files are stored
    LogFilePrefix    = "anthropic-debug"  // Prefix for all log file names
)

// LogServer represents the core logging infrastructure. Each server instance
// manages its own log file and message processing goroutine. The server
// ensures thread-safe operations through mutex synchronization.
type LogServer struct {
    mu       sync.Mutex      // Mutex for protecting server state changes
    file     *os.File        // Handle to the current log file
    enabled  bool            // Flag indicating if logging is active
    logChan  chan string     // Channel for passing messages to the logging goroutine
    done     chan struct{}   // Channel for signaling when the logging goroutine has finished
    bufSize  int            // Size of the message buffer channel
}

// LogServerOption defines a function type for modifying LogServer settings
// during initialization. This follows the functional options pattern.
type LogServerOption func(*LogServer)

// Global variables for managing the singleton server instance.
// The mutex protects access to the server pointer during initialization
// and shutdown operations.
var (
    server *LogServer    // Singleton server instance
    mu     sync.Mutex    // Global mutex for protecting server creation/access
)

// WithBufferSize creates an option to configure the size of the logging
// channel buffer. This allows users to adjust the buffer size based on
// their specific logging volume requirements.
func WithBufferSize(size int) LogServerOption {
    return func(s *LogServer) {
        if size > 0 {
            s.bufSize = size
        }
    }
}

// getServer safely retrieves the current server instance, creating
// a new one if necessary. This function ensures thread-safe access
// to the global server instance.
func getServer() *LogServer {
    mu.Lock()
    if server == nil {
        s := &LogServer{
            bufSize: DefaultBufferSize,
            done:    make(chan struct{}),
        }
        s.logChan = make(chan string, s.bufSize)
        server = s
    }
    s := server
    mu.Unlock()
    return s
}

// InitLogServer initializes or reinitializes the logging system with
// the provided options. If a server already exists, it's shut down
// gracefully before the new server is created.
func InitLogServer(opts ...LogServerOption) {
    mu.Lock()
    // If we have an existing server, shut it down first
    if server != nil {
        old := server
        server = nil
        mu.Unlock() // Release global lock before shutdown
        
        // Clean up old server if it exists
        if old != nil {
            old.mu.Lock()
            if old.enabled {
                old.enabled = false
                close(old.logChan)
                old.mu.Unlock()
                <-old.done // Wait for processor to complete
                
                old.mu.Lock()
                if old.file != nil {
                    fmt.Fprintf(old.file, "\n=== Session Ended: %s ===\n",
                        time.Now().Format("2006-01-02 15:04:05"))
                    old.file.Close()
                }
                old.mu.Unlock()
            } else {
                old.mu.Unlock()
            }
        }
    } else {
        mu.Unlock()
    }
    
    // Create and configure new server
    s := &LogServer{
        bufSize: DefaultBufferSize,
        done:    make(chan struct{}),
    }
    
    // Apply any provided options
    for _, opt := range opts {
        opt(s)
    }
    
    s.logChan = make(chan string, s.bufSize)
    
    // Install new server
    mu.Lock()
    server = s
    mu.Unlock()
}

// EnableLogging activates the logging system, creating necessary
// directories and files. It's safe to call multiple times - subsequent
// calls will return nil if logging is already enabled.
func EnableLogging() error {
    s := getServer()
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.enabled {
        return nil
    }

    // Ensure log directory exists
    if err := os.MkdirAll(LogDirName, 0755); err != nil {
        return fmt.Errorf("failed to create logs directory: %w", err)
    }

    // Create new log file with timestamp
    timestamp := time.Now().Format("20060102-150405")
    filename := filepath.Join(LogDirName, fmt.Sprintf("%s-%s.log", LogFilePrefix, timestamp))

    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create log file: %w", err)
    }

    s.file = file
    s.enabled = true
    
    // Start log processor goroutine
    go s.processLogs()
    
    // Write session start marker
    fmt.Fprintf(s.file, "=== Session Started: %s ===\n\n", 
        time.Now().Format("2006-01-02 15:04:05"))
    
    return nil
}

// DisableLogging gracefully shuts down the logging system. It ensures
// all pending messages are written before closing the log file.
func DisableLogging() {
    s := getServer()
    s.mu.Lock()
    if !s.enabled {
        s.mu.Unlock()
        return
    }

    s.enabled = false
    close(s.logChan)
    s.mu.Unlock()

    <-s.done // Wait for processor to finish

    s.mu.Lock()
    if s.file != nil {
        fmt.Fprintf(s.file, "\n=== Session Ended: %s ===\n",
            time.Now().Format("2006-01-02 15:04:05"))
        s.file.Close()
        s.file = nil
    }
    s.mu.Unlock()
}

// processLogs is the main logging goroutine that processes messages
// from the log channel and writes them to the file. It runs until
// the log channel is closed.
func (s *LogServer) processLogs() {
    defer close(s.done)
    for text := range s.logChan {
        s.mu.Lock()
        if s.file != nil {
            timestamp := time.Now().Format("2006-01-02 15:04:05.000")
            fmt.Fprintf(s.file, "[%s] %s\n", timestamp, text)
        }
        s.mu.Unlock()
    }
}

// WriteLogs asynchronously writes a message to the log file. If the
// logging system is disabled or the message buffer is full, the
// message will be dropped.
func WriteLogs(text string) {
    s := getServer()
    if !s.enabled {
        return
    }
    
    select {
    case s.logChan <- text:
        // Message sent successfully
    default:
        // Channel is full, log a warning to stderr
        fmt.Fprintf(os.Stderr, "Warning: Log channel full, message dropped: %s\n", text)
    }
}

// IsLoggingEnabled returns whether the logging system is currently
// active and accepting messages.
func IsLoggingEnabled() bool {
    s := getServer()
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.enabled
}
