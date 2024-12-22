# Logging System

A thread-safe, asynchronous logging system that writes messages to timestamped files.

## Installation

```go
import "path/to/logging"
```

## Quick Start

```go
// Initialize with default settings (buffer size: 1000)
InitLogServer()

// Enable logging - creates ./logs directory and log file
if err := EnableLogging(); err != nil {
    log.Fatal(err)
}

// Write logs asynchronously
WriteLogs("Starting application...")

// Check logging status
if IsLoggingEnabled() {
    WriteLogs("System is running")
}

// Disable logging - ensures all messages are written before closing
DisableLogging()
```

## Custom Configuration

```go
// Initialize with custom buffer size
InitLogServer(WithBufferSize(2000))
```

## Features

- Thread-safe operations
- Non-blocking writes with configurable buffer
- Automatic log file creation with timestamps
- Clean session markers
- Log files format: `./logs/anthropic-debug-YYYYMMDD-HHMMSS.log`

## Best Practices

1. Initialize early in your application
2. Always handle `EnableLogging()` errors
3. Call `DisableLogging()` before application shutdown
4. Monitor stderr for dropped message warnings

## File Format Example

```
=== Session Started: 2024-12-09 15:04:05 ===
[2024-12-09 15:04:05.123] Log message 1
[2024-12-09 15:04:06.234] Log message 2
=== Session Ended: 2024-12-09 15:04:07 ===
```
