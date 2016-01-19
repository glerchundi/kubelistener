package logrus

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"io"
)

var MainLogger = NewLogger()

func init() {
	MainLogger.SetTag(os.Args[0])
}

type Formatter interface {
	Format(tag, level, message string) string
}

type DefaultFormatter struct {
}

func (dlf *DefaultFormatter) Format(tag, level, message string) string {
	timestamp := time.Now().Format(time.RFC3339)
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s %s %s[%d]: %s %s\n", timestamp, hostname, tag, os.Getpid(), level, message)
}

type Logger struct {
	log *logrus.Logger
	tag string
	formatter Formatter
}

type logrusFormatter struct {
	logger *Logger
}

func (l *logrusFormatter) Format(e *logrus.Entry) ([]byte, error) {
	return []byte(l.logger.formatter.Format(l.logger.tag, strings.ToUpper(e.Level.String()), e.Message)), nil
}

func NewLogger() *Logger {
	logger := &Logger{log:logrus.New(), formatter:&DefaultFormatter{}}
	logger.log.Formatter = &logrusFormatter{logger}
	return logger
}

func (l *Logger) SetTag(tag string) {
	l.tag = tag
}

func (l *Logger) SetLevel(level string) {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		Fatal(fmt.Sprintf(`not a valid level: "%s"`, level))
	}
	l.log.Level = lvl
}

func (l *Logger) SetOutput(w io.Writer) {
	l.log.Out = w
}

func (l *Logger) SetFormatter(f Formatter) {
	l.formatter = f
}

func (l *Logger) Debugf(format string, args ...interface{}) { l.log.Debugf(format, args...) }
func (l *Logger) Infof(format string, args ...interface{}) { l.log.Infof(format, args...) }
func (l *Logger) Printf(format string, args ...interface{}) { l.log.Printf(format, args...) }
func (l *Logger) Warnf(format string, args ...interface{}) { l.log.Warnf(format, args...) }
func (l *Logger) Errorf(format string, args ...interface{}) { l.log.Errorf(format, args...) }
func (l *Logger) Fatalf(format string, args ...interface{}) { l.log.Fatalf(format, args...) }
func (l *Logger) Panicf(format string, args ...interface{}) { l.log.Panicf(format, args...) }

func (l *Logger) Debug(args ...interface{}) { l.log.Debug(args...) }
func (l *Logger) Info(args ...interface{}) { l.log.Info(args...) }
func (l *Logger) Print(args ...interface{}) { l.log.Print(args...) }
func (l *Logger) Warn(args ...interface{}) { l.log.Warn(args...) }
func (l *Logger) Error(args ...interface{}) { l.log.Error(args...) }
func (l *Logger) Fatal(args ...interface{}) { l.log.Fatal(args...) }
func (l *Logger) Panic(args ...interface{}) { l.log.Panic(args...) }

func (l *Logger) Debugln(args ...interface{}) { l.log.Debugln(args...) }
func (l *Logger) Infoln(args ...interface{}) { l.log.Infoln(args...) }
func (l *Logger) Println(args ...interface{}) { l.log.Println(args...) }
func (l *Logger) Warnln(args ...interface{}) { l.log.Warnln(args...) }
func (l *Logger) Errorln(args ...interface{}) { l.log.Errorln(args...) }
func (l *Logger) Fatalln(args ...interface{}) { l.log.Fatalln(args...) }
func (l *Logger) Panicln(args ...interface{}) { l.log.Panicln(args...) }

func Debugf(format string, args ...interface{}) { MainLogger.Debugf(format, args...) }
func Infof(format string, args ...interface{}) { MainLogger.Infof(format, args...) }
func Printf(format string, args ...interface{}) { MainLogger.Printf(format, args...) }
func Warnf(format string, args ...interface{}) { MainLogger.Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { MainLogger.Errorf(format, args...) }
func Fatalf(format string, args ...interface{}) { MainLogger.Fatalf(format, args...) }
func Panicf(format string, args ...interface{}) { MainLogger.Panicf(format, args...) }

func Debug(args ...interface{}) { MainLogger.Debug(args...) }
func Info(args ...interface{}) { MainLogger.Info(args...) }
func Print(args ...interface{}) { MainLogger.Print(args...) }
func Warn(args ...interface{}) { MainLogger.Warn(args...) }
func Error(args ...interface{}) { MainLogger.Error(args...) }
func Fatal(args ...interface{}) { MainLogger.Fatal(args...) }
func Panic(args ...interface{}) { MainLogger.Panic(args...) }

func Debugln(args ...interface{}) { MainLogger.Debugln(args...) }
func Infoln(args ...interface{}) { MainLogger.Infoln(args...) }
func Println(args ...interface{}) { MainLogger.Println(args...) }
func Warnln(args ...interface{}) { MainLogger.Warnln(args...) }
func Errorln(args ...interface{}) { MainLogger.Errorln(args...) }
func Fatalln(args ...interface{}) { MainLogger.Fatalln(args...) }
func Panicln(args ...interface{}) { MainLogger.Panicln(args...) }