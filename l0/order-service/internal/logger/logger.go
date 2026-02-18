package logger

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	Gray   = "\033[37m"
)

var mu sync.Mutex

// timestamp returns current time formatted for logs
func timestamp() string {
	return time.Now().Format("2006/01/02 15:04:05")
}

// write outputs directly to stderr (unbuffered)
func write(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	mu.Lock()
	fmt.Fprint(os.Stderr, msg)
	mu.Unlock()
}

func Info(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	write("%s %sℹ️  %s%s\n", timestamp(), Blue, msg, Reset)
}

func Success(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	write("%s %s✅ %s%s\n", timestamp(), Green, msg, Reset)
}

func Warn(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	write("%s %s⚠️  %s%s\n", timestamp(), Yellow, msg, Reset)
}

func Error(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	write("%s %s❌ %s%s\n", timestamp(), Red, msg, Reset)
}

func Title(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	write("\n%s %s=== %s ===%s\n", timestamp(), Purple, strings.ToUpper(msg), Reset)
}

// ValidationErr prints errors in a vertical list
func ValidationErr(context string, errs []string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n%s %s🚫 VALIDATION FAILED for %s:%s\n", timestamp(), Red, context, Reset))

	for _, e := range errs {
		sb.WriteString(fmt.Sprintf("   %s• %s%s\n", Yellow, e, Reset))
	}
	sb.WriteString("\n")

	write("%s", sb.String())
}
