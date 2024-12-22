# Logging Server Implementation Documentation

## Overview
The logging server provides asynchronous, thread-safe logging capabilities for the Anthropic client. It manages log file creation, writing, and cleanup through a channel-based architecture.

## Core Components

### LogServer Structure
```go
type LogServer struct {
    mu       sync.Mutex    // Synchronization for thread safety
    file     *os.File      // Current log file handle
    enabled  bool          // Logging state flag
    logChan  chan string   // Message buffer channel
    done     chan struct{} // Shutdown synchronization
    bufSize  int          // Channel buffer size
}
```

### Configuration
- Default buffer size: 1000 messages
- Log directory: `./logs`
- File naming: `anthropic-debug-YYYYMMDD-HHMMSS.log`
- Configurable via options pattern: `WithBufferSize()`

## Key Features

### Thread Safety
- Mutex-protected file operations
- Single writer goroutine design
- Synchronized shutdown process

### Non-Blocking Writes
```go
select {
case s.logChan <- text:
    // Message sent successfully
default:
    // Channel full, message dropped with warning
}
```

### Session Management
- Automatic session markers
- Timestamped entries
- Clean shutdown handling

## Usage Examples

### Initialization
```go
// Default configuration
InitLogServer()

// Custom buffer size
InitLogServer(WithBufferSize(2000))
```

### Basic Operations
```go
// Start logging
EnableLogging()

// Write logs
WriteLogs("Operation started")

// Check status
if IsLoggingEnabled() {
    // Logging-dependent code
}

// Stop logging
DisableLogging()
```

## File Format
```
=== Session Started: 2024-12-09 15:04:05 ===
[2024-12-09 15:04:05.123] Log message 1
[2024-12-09 15:04:06.234] Log message 2
=== Session Ended: 2024-12-09 15:04:07 ===
```

## Error Handling
- Directory creation failures
- File operation errors
- Channel overflow conditions

## Performance Considerations
1. Buffer size impacts memory usage
2. Non-blocking writes prevent application slowdown
3. Single writer eliminates contention
4. Mutex scope minimized for better concurrency

## Best Practices
1. Initialize early in application lifecycle
2. Handle EnableLogging errors
3. Call DisableLogging before shutdown
4. Monitor channel overflow warnings
5. Use appropriate buffer sizes for workload
