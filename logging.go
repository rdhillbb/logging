package logging

import (
   "bytes"
   "fmt"
   "os"
   "path/filepath"
   "runtime"
   "sync"
   "sync/atomic"
   "time"
)

const (
   defaultBufferSize   = 20000    
   defaultNumWorkers   = 8
   defaultBatchSize    = 10000
   defaultFlushIntervalMs = 50
   maxFileSize        = 1 << 30  // 1GB
   maxFiles          = 10
   logDirName        = "logs"
   logFilePrefix     = "anthropic-debug"
)

type LogLevel int32

const (
   DEBUG LogLevel = iota
   INFO
   WARN  
   ERROR
   FATAL
)

type LogServer struct {
   mu              sync.RWMutex
   currentFile     *os.File
   fileSize        int64
   enabled         atomic.Bool
   logChans        []chan logMessage
   buffers         []*bytes.Buffer
   numWorkers      int
   batchSize       int
   flushInterval   time.Duration
   logLevel        LogLevel
   logDir          string
   rotateSize      int64
   maxFiles        int
}

type logMessage struct {
   timestamp time.Time
   level     LogLevel
   file      string
   line      int
   text      string
}

var (
   server *LogServer
   once   sync.Once
)

type Config struct {
   NumWorkers    int
   BatchSize     int
   FlushInterval time.Duration
   LogLevel      LogLevel  
   LogDir        string
   RotateSize    int64
   MaxFiles      int
}

func DefaultConfig() Config {
   return Config{
       NumWorkers:    defaultNumWorkers,
       BatchSize:     defaultBatchSize, 
       FlushInterval: time.Duration(defaultFlushIntervalMs) * time.Millisecond,
       LogLevel:      INFO,
       LogDir:        logDirName,
       RotateSize:    maxFileSize,
       MaxFiles:      maxFiles,
   }
}

func getServer() *LogServer {
   once.Do(func() {
       server = NewLogServer(DefaultConfig())
   })
   return server
}

func NewLogServer(config Config) *LogServer {
   s := &LogServer{
       logChans:      make([]chan logMessage, config.NumWorkers),
       buffers:       make([]*bytes.Buffer, config.NumWorkers),
       numWorkers:    config.NumWorkers,
       batchSize:     config.BatchSize,
       flushInterval: config.FlushInterval,
       logLevel:      config.LogLevel,
       logDir:        config.LogDir,
       rotateSize:    config.RotateSize,
       maxFiles:      config.MaxFiles,
   }

   for i := 0; i < config.NumWorkers; i++ {
       s.logChans[i] = make(chan logMessage, defaultBufferSize)
       s.buffers[i] = bytes.NewBuffer(make([]byte, 0, 1<<24))
       go s.processWorker(i)
   }

   go s.periodicFlush()
   return s
}

func (s *LogServer) processWorker(id int) {
   buffer := s.buffers[id]
   count := 0

   for msg := range s.logChans[id] {
       if !s.enabled.Load() {
           continue
       }

       if msg.level < s.logLevel {
           continue  
       }

       buffer.WriteString(fmt.Sprintf("[%s] [%s] %s:%d %s\n",
           msg.timestamp.Format("2006-01-02 15:04:05.000"),
           levelToString(msg.level),
           msg.file,
           msg.line,
           msg.text))
       
       count++
       if count >= s.batchSize {
           s.flush(id)
           count = 0
       }
   }
}

func (s *LogServer) periodicFlush() {
   ticker := time.NewTicker(s.flushInterval)
   defer ticker.Stop()

   for range ticker.C {
       if !s.enabled.Load() {
           continue
       }
       s.flushAll()
   }
}

func (s *LogServer) flush(id int) {
   if s.buffers[id].Len() == 0 {
       return
   }

   s.mu.Lock()
   defer s.mu.Unlock()

   if s.currentFile == nil {
       return
   }

   data := s.buffers[id].Bytes()
   n, err := s.currentFile.Write(data)
   if err != nil {
       fmt.Fprintf(os.Stderr, "Error writing to log file: %v\n", err)
       return
   }

   s.fileSize += int64(n)
   s.buffers[id].Reset()

   if s.fileSize >= s.rotateSize {
       s.rotate()
   }
}

func (s *LogServer) flushAll() {
   for i := 0; i < s.numWorkers; i++ {
       s.flush(i)
   }
   if s.currentFile != nil {
       s.currentFile.Sync()
   }
}

func (s *LogServer) rotate() {
   if s.currentFile != nil {
       s.currentFile.Close()
   }

   // Delete old files if we have too many
   files, err := filepath.Glob(filepath.Join(s.logDir, fmt.Sprintf("%s-*.log", logFilePrefix)))
   if err == nil && len(files) >= s.maxFiles {
       for i := 0; i < len(files)-s.maxFiles+1; i++ {
           os.Remove(files[i])
       }
   }

   // Create new file
   timestamp := time.Now().Format("20060102-150405")
   newPath := filepath.Join(s.logDir, fmt.Sprintf("%s-%s.log", logFilePrefix, timestamp))
   
   file, err := os.Create(newPath)
   if err != nil {
       fmt.Fprintf(os.Stderr, "Error creating new log file: %v\n", err)
       return
   }

   s.currentFile = file
   s.fileSize = 0
}

func EnableLogging() error {
   s := getServer()
   
   if err := os.MkdirAll(s.logDir, 0755); err != nil {
       return fmt.Errorf("failed to create log directory: %w", err)
   }

   s.mu.Lock()
   defer s.mu.Unlock()

   if s.enabled.Load() {
       return nil
   }

   s.rotate() // Create initial file
   s.enabled.Store(true)
   
   return nil
}

func DisableLogging() {
   s := getServer()
   s.enabled.Store(false)
   
   s.mu.Lock()
   defer s.mu.Unlock()

   s.flushAll()
   if s.currentFile != nil {
       s.currentFile.Close()
       s.currentFile = nil
   }
}

func IsLoggingEnabled() bool {
   return getServer().enabled.Load()
}

func SetLogLevel(level LogLevel) {
   getServer().logLevel = level
}

func levelToString(level LogLevel) string {
   switch level {
   case DEBUG:
       return "DEBUG"
   case INFO:
       return "INFO"
   case WARN:
       return "WARN"
   case ERROR:
       return "ERROR"
   case FATAL:
       return "FATAL"
   default:
       return "UNKNOWN"
   }
}

func getCallerInfo() (string, int) {
   _, file, line, ok := runtime.Caller(2)
   if !ok {
       return "unknown", 0
   }
   return filepath.Base(file), line
}

func logWithLevel(level LogLevel, text string) {
   s := getServer()
   if !s.enabled.Load() || level < s.logLevel {
       return
   }

   file, line := getCallerInfo()
   msg := logMessage{
       timestamp: time.Now(),
       level:     level,
       file:      file,
       line:      line,
       text:      text,
   }

   workerID := time.Now().UnixNano() % int64(s.numWorkers)
   select {
   case s.logChans[workerID] <- msg:
   default:
       fmt.Fprintf(os.Stderr, "Warning: Log channel full, message dropped: %s\n", text)
   }
}

func Debug(text string) { logWithLevel(DEBUG, text) }
func Info(text string)  { logWithLevel(INFO, text) }
func Warn(text string)  { logWithLevel(WARN, text) }
func Error(text string) { logWithLevel(ERROR, text) }
func Fatal(text string) { 
   logWithLevel(FATAL, text)
   os.Exit(1)
}

// Backward compatibility
func WriteLogs(text string) { Info(text) }
