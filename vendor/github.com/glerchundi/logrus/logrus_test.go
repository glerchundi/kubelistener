package logrus

import (
    "bytes"
    "testing"

    "github.com/stretchr/testify/assert"
)

const (
    testString = "f9VfzZvBGdCuKq2QgdPu3OAKvnktQXOVl3GYMyRXUnnuG66YP7VFtdWx5HR6"
)

func TestMainLogger(t *testing.T) {
    var buffer bytes.Buffer
    MainLogger.SetOutput(&buffer)
    Info(testString)
    assert.Contains(t, buffer.String(), testString)
}

func TestInstantiatedLogger(t *testing.T) {
    var buffer bytes.Buffer
    l := NewLogger()
    l.SetOutput(&buffer)
    l.Info(testString)
    assert.Contains(t, buffer.String(), testString)
}