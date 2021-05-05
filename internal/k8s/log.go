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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/bombsimon/logrusr"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	klog "k8s.io/klog/v2"
)

type klogParams struct {
	flags *flag.FlagSet
	log   *logrus.Entry
}

type LogOption func(*klogParams)

// LogLevelOption creates an option to set the Kubernetes verbose
// log level (1 - 10 is the standard range).
func LogLevelOption(level int) LogOption {
	return LogOption(func(p *klogParams) {
		if err := p.flags.Set("v", strconv.Itoa(level)); err != nil {
			panic(fmt.Sprintf("failed to set flag: %s", err))
		}
	})
}

// LogWriterOption creates an option to set the Kubernetes logging output.
func LogWriterOption(log *logrus.Entry) LogOption {
	return LogOption(func(p *klogParams) {
		p.log = log
	})
}

// InitLogging initializes the Kubernetes client-go logging subsystem.
func InitLogging(options ...LogOption) {
	must := func(err error) {
		if err != nil {
			panic(err.Error())
		}
	}

	p := klogParams{
		flags: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}

	// First, init the flags so that we can set specific values.
	klog.InitFlags(p.flags)

	for _, o := range options {
		o(&p)
	}

	switch p.log {
	case nil:
	default:
		// Force klog to a file output so that it uses the PipeWriter.
		must(p.flags.Set("logtostderr", "false"))
		must(p.flags.Set("alsologtostderr", "false"))

		klog.SetLogger(&callDepthLogr{
			Logger: logrusr.NewLogger(p.log),
		})
	}
}

type callDepthLogr struct {
	logr.Logger
	callDepth int
}

func (l *callDepthLogr) WithCallDepth(depth int) logr.Logger {
	return &callDepthLogr{
		Logger:    l.Logger,
		callDepth: depth,
	}
}

func (l *callDepthLogr) Info(msg string, keysAndValues ...interface{}) {
	_, file, line, ok := runtime.Caller(l.callDepth + 1)
	if ok {
		keysAndValues = append(keysAndValues, "location", fmt.Sprintf("%s:%d", filepath.Base(file), line))
	}
	l.Logger.Info(msg, keysAndValues...)
}

func (l *callDepthLogr) Error(err error, msg string, keysAndValues ...interface{}) {
	_, file, line, ok := runtime.Caller(l.callDepth + 1)
	if ok {
		keysAndValues = append(keysAndValues, "location", fmt.Sprintf("%s:%d", filepath.Base(file), line))
	}
	l.Logger.Error(err, msg, keysAndValues...)
}

// Override V and just pass through l since we can rely on klog itself to do log
// level filtering.
func (l *callDepthLogr) V(level int) logr.Logger {
	return l
}
