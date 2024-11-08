// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	klog "k8s.io/klog/v2"
	controller_runtime_log "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// klog automatic flush interval is 5s but we can wait for less time to pass
	// since we proactively call klog.Flush().
	klogFlushWaitTime     = time.Millisecond * 100
	klogFlushWaitInterval = time.Millisecond * 1
)

func TestKlogOnlyLogsToLogrus(t *testing.T) {
	// Save stderr/out.
	oldStderr := os.Stderr
	oldStdout := os.Stdout
	// Make sure to reset stderr/out.
	defer func() {
		os.Stderr = oldStderr
		os.Stdout = oldStdout
	}()

	// Create pipes.
	seReader, seWriter, err := os.Pipe()
	require.NoError(t, err)
	soReader, soWriter, err := os.Pipe()
	require.NoError(t, err)

	// Set stderr/out to pipe write end.
	os.Stderr = seWriter
	os.Stdout = soWriter

	// Channel to receive output.
	outC := make(chan string)
	go func() {
		// Close readers on exit.
		defer seReader.Close()
		defer soReader.Close()

		// Read stdout/err together.
		reader := bufio.NewReader(io.MultiReader(seReader, soReader))

		for {
			line, err := reader.ReadString('\n')
			switch err {
			case nil:
				// Send log to channel.
				outC <- line
			case io.EOF:
				// Close channel to ensure test continues.
				close(outC)
				return
			default:
				return
			}
		}
	}()

	log, logHook := test.NewNullLogger()
	InitLogging(LogWriterOption(log.WithField("foo", "bar")))

	infoLog := "some log"
	errorLog := "some error log"
	errorLogged := errors.New("some error")

	// Keep these lines together.
	_, file, line, ok := runtime.Caller(0)
	require.True(t, ok)
	klog.Info(infoLog)
	klog.ErrorS(errorLogged, errorLog)
	klog.Flush()
	sourceFile := filepath.Base(file)
	infoLine := line + 2
	errorLine := line + 3

	// Should be a recorded logrus log with the correct fields.
	require.Eventually(t, func() bool { return len(logHook.AllEntries()) == 2 }, klogFlushWaitTime, klogFlushWaitInterval)

	// Close write end of pipes.
	seWriter.Close()
	soWriter.Close()

	// Stderr/out should be empty.
	assert.Empty(t, <-outC)

	infoEntry := logHook.AllEntries()[0]
	assert.Equal(t, infoLog, infoEntry.Message)
	assert.Len(t, infoEntry.Data, 2)
	assert.Equal(t, "bar", infoEntry.Data["foo"])
	assert.Equal(t, fmt.Sprintf("%s:%d", sourceFile, infoLine), infoEntry.Data["caller"])

	errorEntry := logHook.AllEntries()[1]
	assert.Equal(t, errorLog, errorEntry.Message)
	assert.Len(t, errorEntry.Data, 3)
	assert.Equal(t, "bar", errorEntry.Data["foo"])
	assert.Equal(t, errorLogged, errorEntry.Data["error"])
	assert.Equal(t, fmt.Sprintf("%s:%d", sourceFile, errorLine), errorEntry.Data["caller"])
}

func TestControllerRuntimeLoggerLogsToLogrus(t *testing.T) {
	// Comment out the following line to run this test.
	t.Skip("this test has to be run individually since the controller-runtime logging infrastructure can only be initialized once per process")

	log, logHook := test.NewNullLogger()
	InitLogging(LogWriterOption(log.WithField("foo", "bar")))

	controller_runtime_log.Log.Info("some message")
	require.Eventually(t, func() bool { return len(logHook.AllEntries()) == 1 }, klogFlushWaitTime, klogFlushWaitInterval)
	assert.Equal(t, "some message", logHook.AllEntries()[0].Message)
}

// Last LogWriterOption passed in should be used.
func TestMultipleLogWriterOptions(t *testing.T) {
	log, logHook := test.NewNullLogger()
	logEntry1 := log.WithField("field", "data1")
	logEntry2 := log.WithField("field", "data2")
	logEntry3 := log.WithField("field", "data3")
	InitLogging(LogWriterOption(logEntry1), LogWriterOption(logEntry2), LogWriterOption(logEntry3))

	klog.Info("some log")
	klog.Flush()
	require.Eventually(t, func() bool { return len(logHook.AllEntries()) == 1 }, klogFlushWaitTime, klogFlushWaitInterval)
	assert.Equal(t, "data3", logHook.AllEntries()[0].Data["field"])
}

func TestLogLevelOptionKlog(t *testing.T) {
	log, _ := test.NewNullLogger()
	l := log.WithField("some", "field")
	for logLevel := 0; logLevel <= 10; logLevel++ {
		t.Run(fmt.Sprintf("log level %d", logLevel), func(t *testing.T) {
			InitLogging(LogWriterOption(l), LogLevelOption(logLevel))
			// Make sure log verbosity is set properly.
			for verbosityLevel := 0; verbosityLevel <= 10; verbosityLevel++ {
				enabled := klog.V(klog.Level(verbosityLevel)).Enabled() //nolint:gosec // disable G115
				if verbosityLevel <= logLevel {
					assert.True(t, enabled)
				} else {
					assert.False(t, enabled)
				}
			}
		})
	}
}

func TestLogLevelOptionControllerRuntime(t *testing.T) {
	// Comment out the following line to run this test.
	t.Skip("this test has to be run individually since the controller-runtime logging infrastructure can only be initialized once per process")

	log, logHook := test.NewNullLogger()
	l := log.WithField("some", "field")

	// We can only call InitLogging once and test the output of the
	// controller-runtime logger with one log level because the
	// underlying logger does not let us reset it.
	logLevel := 5
	InitLogging(LogWriterOption(l), LogLevelOption(logLevel))
	// Make sure log verbosity is set properly.
	for verbosityLevel := 1; verbosityLevel <= 10; verbosityLevel++ {
		enabled := controller_runtime_log.Log.V(verbosityLevel).Enabled()
		if verbosityLevel <= logLevel {
			assert.True(t, enabled)
			controller_runtime_log.Log.V(verbosityLevel).Info("something")
			assert.Eventually(t, func() bool { return len(logHook.AllEntries()) == 1 }, klogFlushWaitTime, klogFlushWaitInterval)
		} else {
			assert.False(t, enabled)
			controller_runtime_log.Log.V(verbosityLevel).Info("something")
			assert.Never(t, func() bool { return len(logHook.AllEntries()) > 0 }, klogFlushWaitTime, klogFlushWaitInterval)
		}
		logHook.Reset()
	}
}

func TestMultipleLogLevelOptions(t *testing.T) {
	log, _ := test.NewNullLogger()
	l := log.WithField("some", "field")
	InitLogging(LogWriterOption(l), LogLevelOption(1), LogLevelOption(10), LogLevelOption(4))
	assert.True(t, klog.V(3).Enabled())
	assert.False(t, klog.V(5).Enabled())
}
