package go_kcp

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type KcpLogType int

var KcpLogMask int64

func SetKcpLogMask(mask int) {
	atomic.StoreInt64(&KcpLogMask, int64(mask))
	return
}

const (
	IKCP_LOG_OUTPUT    KcpLogType = 1
	IKCP_LOG_INPUT     KcpLogType = 2
	IKCP_LOG_SEND      KcpLogType = 4
	IKCP_LOG_RECV      KcpLogType = 8
	IKCP_LOG_IN_DATA   KcpLogType = 16
	IKCP_LOG_IN_ACK    KcpLogType = 32
	IKCP_LOG_IN_PROBE  KcpLogType = 64
	IKCP_LOG_IN_WINS   KcpLogType = 128
	IKCP_LOG_OUT_DATA  KcpLogType = 256
	IKCP_LOG_OUT_ACK   KcpLogType = 512
	IKCP_LOG_OUT_PROBE KcpLogType = 1024
	IKCP_LOG_OUT_WINS  KcpLogType = 2048
)

var (
	logMap = map[KcpLogType]string{
		IKCP_LOG_OUTPUT:    "log_output",
		IKCP_LOG_INPUT:     "log_input",
		IKCP_LOG_SEND:      "log_send",
		IKCP_LOG_RECV:      "log_recv",
		IKCP_LOG_IN_DATA:   "log_in_data",
		IKCP_LOG_IN_ACK:    "log_in_ack",
		IKCP_LOG_IN_PROBE:  "log_in_probe",
		IKCP_LOG_IN_WINS:   "log_in_wins",
		IKCP_LOG_OUT_DATA:  "log_in_out_data",
		IKCP_LOG_OUT_ACK:   "log_in_out_ack",
		IKCP_LOG_OUT_PROBE: "log_in_out_probe",
		IKCP_LOG_OUT_WINS:  "log_in_out_wins",
	}
)

// Colors
const (
	Reset       = "\033[0m"
	Red         = "\033[31m"
	Green       = "\033[32m"
	Yellow      = "\033[33m"
	Blue        = "\033[34m"
	Magenta     = "\033[35m"
	Cyan        = "\033[36m"
	White       = "\033[37m"
	BlueBold    = "\033[34;1m"
	MagentaBold = "\033[35;1m"
	RedBold     = "\033[31;1m"
	YellowBold  = "\033[33;1m"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelError
	LogLevelWarn
	LogLevelSilent
)

var (
	DebugStr = Blue + " %s " + Reset + Blue + "[debug] " + Reset
	infoStr  = Green + " %s " + Reset + Green + "[info] " + Reset
	warnStr  = BlueBold + " %s " + Reset + Magenta + "[warn] " + Reset
	errStr   = Magenta + " %s " + Reset + Red + "[error] " + Reset
)

var kcpSourceDir string

func init() {
	_, file, _, _ := runtime.Caller(0)
	// compatible solution to get gorm source directory with various operating systems
	kcpSourceDir = sourceDir(file)
	SetKcpLogMask(1<<33 - 1)
	SetLogLevel(LogLevelWarn)
}

func sourceDir(file string) string {
	dir := filepath.Dir(file)
	dir = filepath.Dir(dir)

	s := filepath.Dir(dir)
	s = dir
	return filepath.ToSlash(s) + "/"
}

func FileWithLineNum() string {
	for i := 2; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i)
		if ok && (strings.HasPrefix(file, kcpSourceDir) || strings.HasSuffix(file, "_test.go")) {
			return file + ":" + strconv.FormatInt(int64(line), 10)
		}
	}

	return ""
}

var (
	logLevel int32
)

func SetLogLevel(level LogLevel) {
	atomic.StoreInt32(&logLevel, int32(level))
}

func GetLogLevel() LogLevel {
	return LogLevel(atomic.LoadInt32(&logLevel))
}

func Debug(msg string, data ...interface{}) {
	if GetLogLevel() <= LogLevelDebug {
		printMsg(DebugStr+msg, append([]interface{}{FileWithLineNum()}, data...))
	}
}

func Info(msg string, data ...interface{}) {
	if GetLogLevel() <= LogLevelInfo {
		printMsg(infoStr+msg, append([]interface{}{FileWithLineNum()}, data...))
	}
}

func Warn(msg string, data ...interface{}) {
	if GetLogLevel() <= LogLevelWarn {
		printMsg(warnStr+msg, append([]interface{}{FileWithLineNum()}, data...))
	}
}

func Error(msg string, data ...interface{}) {
	if GetLogLevel() <= LogLevelError {
		printMsg(errStr+msg, append([]interface{}{FileWithLineNum()}, data...))
	}
}

func printMsg(msg string, data []interface{}) {
	for i, v := range data {
		if v1, ok := v.(KcpLogType); ok {
			data[i] = fmt.Sprintf("[ logType = %s]", logMap[v1])
			if atomic.LoadInt64(&KcpLogMask)|int64(v1) == 0 {
				return
			}
		}
	}
	t := Yellow + time.Now().Format("2006-01-02 15:04:05.9999999") + Reset
	fmt.Printf(t+" "+msg, data...)
	fmt.Println()
}
