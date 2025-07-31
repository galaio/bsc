package log

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"time"
	"unsafe"
)

var (
	rootLock sync.RWMutex
	root     Logger
)

func init() {
	root = &logger{slog.New(DiscardHandler())}
}

// SetDefault sets the default global logger
func SetDefault(l Logger) {
	rootLock.Lock()
	defer rootLock.Unlock()

	root = l
	if lg, ok := l.(*logger); ok {
		slog.SetDefault(lg.inner)
	}
}

// Root returns the root logger
func Root() Logger {
	rootLock.RLock()
	defer rootLock.RUnlock()

	return root
}

// The following functions bypass the exported logger methods (logger.Debug,
// etc.) to keep the call depth the same for all paths to logger.Write so
// runtime.Caller(2) always refers to the call site in client code.

// Trace is a convenient alias for Root().Trace
//
// Log a message at the trace level with context key/value pairs
//
// # Usage
//
//	log.Trace("msg")
//	log.Trace("msg", "key1", val1)
//	log.Trace("msg", "key1", val1, "key2", val2)
func Trace(msg string, ctx ...interface{}) {
	Root().Write(LevelTrace, msg, ctx...)
}

// Debug is a convenient alias for Root().Debug
//
// Log a message at the debug level with context key/value pairs
//
// # Usage Examples
//
//	log.Debug("msg")
//	log.Debug("msg", "key1", val1)
//	log.Debug("msg", "key1", val1, "key2", val2)
func Debug(msg string, ctx ...interface{}) {
	Root().Write(slog.LevelDebug, msg, ctx...)
}

// Info is a convenient alias for Root().Info
//
// Log a message at the info level with context key/value pairs
//
// # Usage Examples
//
//	log.Info("msg")
//	log.Info("msg", "key1", val1)
//	log.Info("msg", "key1", val1, "key2", val2)
func Info(msg string, ctx ...interface{}) {
	Root().Write(slog.LevelInfo, msg, ctx...)
}

// Warn is a convenient alias for Root().Warn
//
// Log a message at the warn level with context key/value pairs
//
// # Usage Examples
//
//	log.Warn("msg")
//	log.Warn("msg", "key1", val1)
//	log.Warn("msg", "key1", val1, "key2", val2)
func Warn(msg string, ctx ...interface{}) {
	Root().Write(slog.LevelWarn, msg, ctx...)
}

// Error is a convenient alias for Root().Error
//
// Log a message at the error level with context key/value pairs
//
// # Usage Examples
//
//	log.Error("msg")
//	log.Error("msg", "key1", val1)
//	log.Error("msg", "key1", val1, "key2", val2)
func Error(msg string, ctx ...interface{}) {
	Root().Write(slog.LevelError, msg, ctx...)
}

// Crit is a convenient alias for Root().Crit
//
// Log a message at the crit level with context key/value pairs, and then exit.
//
// # Usage Examples
//
//	log.Crit("msg")
//	log.Crit("msg", "key1", val1)
//	log.Crit("msg", "key1", val1, "key2", val2)
func Crit(msg string, ctx ...interface{}) {
	Root().Write(LevelCrit, msg, ctx...)
	time.Sleep(3 * time.Second)
	os.Exit(1)
}

// New returns a new logger with the given context.
// New is a convenient alias for Root().New
func New(ctx ...interface{}) Logger {
	return Root().With(ctx...)
}

// AsyncLogItem is a log item for the async logger
type AsyncLogItem struct {
	msg  string
	args []interface{}
}

func (l *AsyncLogItem) Format(w io.Writer) {
	w.Write(StringToBytes(l.msg))
	w.Write([]byte{' '})
	for i := 0; i < len(l.args); i += 2 {
		if i+1 >= len(l.args) {
			break
		}
		if i > 0 {
			w.Write([]byte{' '})
		}
		if b, ok := (l.args[i+1]).([]byte); ok {
			fmt.Fprintf(w, "%v=%v", l.args[i], hex.EncodeToString(b))
			continue
		}
		if IsInterfaceNil(l.args[i+1]) {
			fmt.Fprintf(w, "%v=%v", l.args[i], "nil")
			continue
		}
		fmt.Fprintf(w, "%v=%v", l.args[i], l.args[i+1])
	}
}
func StringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{s, len(s)},
	))
}

func IsInterfaceNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	}
	return false
}

type AsyncLogger struct {
	f       *os.File
	logChan chan []AsyncLogItem
	stop    chan struct{}
	buffer  []AsyncLogItem
}

func NewAsyncLogger(path string) *AsyncLogger {
	path = fmt.Sprintf("%v_%v", path, time.Now().Format("2006-01-02_15_04"))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	return &AsyncLogger{
		f:       f,
		logChan: make(chan []AsyncLogItem, 10000),
		stop:    make(chan struct{}),
		buffer:  make([]AsyncLogItem, 0, 100),
	}
}

var LogItemsPool = sync.Pool{
	New: func() interface{} {
		return make([]AsyncLogItem, 0, 10000)
	},
}

func (l *AsyncLogger) Write(msg string, ctx []interface{}) {
	if len(l.buffer) < cap(l.buffer) {
		l.buffer = append(l.buffer, AsyncLogItem{
			msg:  msg,
			args: ctx,
		})
		return
	}
	l.logChan <- l.buffer
	l.buffer = LogItemsPool.Get().([]AsyncLogItem)
	l.buffer = l.buffer[:0]
}

func (l *AsyncLogger) AsyncFlush() {
	for {
		select {
		case items := <-l.logChan:
			for _, item := range items {
				item.Format(l.f)
			}
			l.f.Sync()
			LogItemsPool.Put(items)
		case <-l.stop:
			Info("async logger loop exited", "path", l.f.Name())
			return
		}
	}
}

func (l *AsyncLogger) Start() {
	Info("async logger started", "path", l.f.Name())
	go l.AsyncFlush()
}

func (l *AsyncLogger) Stop() {
	Info("async logger stopped", "path", l.f.Name())
	close(l.stop)
}

var AsyncLoggerRoot = NewAsyncLogger("./tracer.log")

func AsyncLog(msg string, ctx ...interface{}) {
	AsyncLoggerRoot.Write(msg, ctx)
}
