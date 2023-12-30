package jsn_net

import (
	"fmt"
	"os"
	"time"
)

type Logger interface {
	Debug(string, ...any)
	Info(string, ...any)
	Warn(string, ...any)
	Error(string, ...any)
	Fatal(string, ...any)
}

var (
	logger Logger
)

func init() {
	logger = new(defaultFmtLogger)
}

func SetLogger(in Logger) {
	logger = in
}

type defaultFmtLogger struct{}

// Fatal implements Logger.
func (*defaultFmtLogger) Fatal(format string, params ...any) {
	fmt.Printf("[%v][Fatal]"+format+"\n", append(append([]any{}, time.Now().Format("2006-01-02 15:04:05.999999")), params...)...)
	os.Exit(2)
}

// Warn implements Logger.
func (*defaultFmtLogger) Warn(format string, params ...any) {
	fmt.Printf("[%v][Warn]"+format+"\n", append(append([]any{}, time.Now().Format("2006-01-02 15:04:05.999999")), params...)...)
}

func (d defaultFmtLogger) Debug(format string, params ...any) {
	fmt.Printf("[%v][Debug]"+format+"\n", append(append([]any{}, time.Now().Format("2006-01-02 15:04:05.999999")), params...)...)
}

func (d defaultFmtLogger) Panic(format string, params ...any) {
	panic(fmt.Errorf(format, params...))
}

func (d defaultFmtLogger) Info(format string, params ...any) {
	fmt.Printf("[%v][Info]"+format+"\n", append(append([]any{}, time.Now().Format("2006-01-02 15:04:05.999999")), params...)...)
}

func (d defaultFmtLogger) Error(format string, params ...any) {
	fmt.Printf("[%v][Error]"+format+"\n", append(append([]any{}, time.Now().Format("2006-01-02 15:04:05.999999")), params...)...)
}
