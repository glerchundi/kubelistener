/*
Copyright (c) 2013 Kelsey Hightower

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

/*
Package log provides support for logging to stdout and stderr.
Log entries will be logged in the following format:
    timestamp hostname tag[pid]: SEVERITY Message
*/

package log

import (
    "fmt"
    "os"
    "strings"
    "time"

    log "github.com/Sirupsen/logrus"
)

type ConfdFormatter struct {
}

func (c *ConfdFormatter) Format(entry *log.Entry) ([]byte, error) {
    timestamp := time.Now().Format(time.RFC3339)
    hostname, _ := os.Hostname()
    return []byte(fmt.Sprintf("%s %s %s[%d]: %s %s\n", timestamp, hostname, tag, os.Getpid(), strings.ToUpper(entry.Level.String()), entry.Message)), nil
}

// tag represents the application name generating the log message. The tag
// string will appear in all log entires.
var tag string

func init() {
    tag = os.Args[0]
    log.SetOutput(os.Stderr)
    log.SetFormatter(&ConfdFormatter{})
}

// SetTag sets the tag.
func SetTag(t string) {
    tag = t
}

// SetLevel sets the log level. Valid levels are panic, fatal, error, warn, info and debug.
func SetLevel(level string) {
    lvl, err := log.ParseLevel(level)
    if err != nil {
        Fatal(fmt.Sprintf(`not a valid level: "%s"`, level))
    }
    log.SetLevel(lvl)
}

// Debug logs a message with severity DEBUG.
func Debug(msg string) {
    log.Debug(msg)
}

// Error logs a message with severity ERROR.
func Error(msg string) {
    log.Error(msg)
}

// Fatal logs a message with severity ERROR followed by a call to os.Exit().
func Fatal(msg string) {
    log.Fatal(msg)
    os.Exit(1)
}

// Info logs a message with severity INFO.
func Info(msg string) {
    log.Info(msg)
}

// Warning logs a message with severity WARNING.
func Warning(msg string) {
    log.Warning(msg)
}