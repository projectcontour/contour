// Copyright © 2017 Heptio
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

// Package log provides a stdlib log implementation of the Logger interface.
package stdlog

import (
	"fmt"
	"io"
	stdlog "log"

	"github.com/heptio/contour/internal/log"
)

// New returns a new Logger. The stdout and stderr parameters
// refer to the destination writers for information and error messages
// respectively. flags are the flags to provide to the underlying
// stdlib log library. If not provided they default to log.LstdFlags
func New(stdout, stderr io.Writer, flags int) log.Logger {
	if flags == 0 {
		flags = stdlog.LstdFlags
	}

	return &infoLogger{
		errorLogger: &errorLogger{
			Logger: stdlog.New(stderr, "", flags),
		},
		Logger: stdlog.New(stdout, "", flags),
	}
}

type errorLogger struct {
	*stdlog.Logger
}

func (e *errorLogger) Error(args ...interface{}) {
	e.Output(2, fmt.Sprintln(args...))
}

func (e *errorLogger) Errorf(format string, args ...interface{}) {
	e.Output(2, fmt.Sprintf(format, args...))
}

type infoLogger struct {
	*errorLogger
	*stdlog.Logger

	// v is the amount of verbosity to permit.
	v int
}

func (l *infoLogger) Infof(format string, args ...interface{}) {
	l.Output(2, fmt.Sprintf(format, args...))
}

func (l *infoLogger) V(v int) log.InfoLogger {
	return &infoLogger{
		errorLogger: l.errorLogger,
		Logger:      l.Logger,
		v:           v,
	}
}

func (l *infoLogger) WithPrefix(prefix string) log.Logger {
	return &prefixLogger{
		infoLogger: l,
		prefix:     prefix,
	}
}

type prefixLogger struct {
	*infoLogger
	prefix string
}

func (l *prefixLogger) Infof(format string, args ...interface{}) {
	l.Output(2, fmt.Sprintf(l.prefix+": "+format, args...))
}

func (l *prefixLogger) Error(args ...interface{}) {
	l.errorLogger.Output(2, fmt.Sprintln(append([]interface{}{l.prefix + ":"}, args...)...))
}

func (l *prefixLogger) Errorf(format string, args ...interface{}) {
	l.errorLogger.Output(2, fmt.Sprintf(l.prefix+": "+format, args...))
}
