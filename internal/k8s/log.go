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
	"strconv"

	"github.com/bombsimon/logrusr/v4"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	klog "k8s.io/klog/v2"
	controller_runtime_log "sigs.k8s.io/controller-runtime/pkg/log"
)

type logParams struct {
	level int
	flags *flag.FlagSet
	log   *logrus.Entry
}

type LogOption func(*logParams)

// LogLevelOption creates an option to set the Kubernetes verbose
// log level (1 - 10 is the standard range).
func LogLevelOption(level int) LogOption {
	return LogOption(func(p *logParams) {
		p.level = level
		if err := p.flags.Set("v", strconv.Itoa(level)); err != nil {
			panic(fmt.Sprintf("failed to set flag: %s", err))
		}
	})
}

// LogWriterOption creates an option to set the Kubernetes logging output.
func LogWriterOption(log *logrus.Entry) LogOption {
	return LogOption(func(p *logParams) {
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

	p := logParams{
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

		// Use the LogSink from a logrusr Logger, but wrapped in
		// an adapter that always returns true for Enabled() since
		// we rely on klog to do log level filtering.
		logger := logrusr.New(p.log, logrusr.WithReportCaller())
		klog.SetLogger(logr.New(&alwaysEnabledLogSink{
			LogSink: logger.GetSink(),
		}))

		// Also set the controller-runtime logger to the same
		// concrete logger but ensure its output is guarded by
		// the configured V level.
		controller_runtime_log.SetLogger(logr.New(&levelControlledLogSink{
			level:   p.level,
			LogSink: logger.GetSink(),
		}))
	}
}

type levelControlledLogSink struct {
	level int
	logr.LogSink
	logr.CallDepthLogSink
}

func (l *levelControlledLogSink) Enabled(level int) bool {
	return level <= l.level
}

// Satisfy the logr.CallDepthLogSink interface to get location logging.
func (l *levelControlledLogSink) WithCallDepth(depth int) logr.LogSink {
	callDepthLogSink, ok := l.LogSink.(logr.CallDepthLogSink)
	if ok {
		return &levelControlledLogSink{
			level:   l.level,
			LogSink: callDepthLogSink.WithCallDepth(depth),
		}
	}

	return l
}

type alwaysEnabledLogSink struct {
	logr.LogSink
	logr.CallDepthLogSink
}

// Satisfy the logr.CallDepthLogSink interface to get location logging.
func (l *alwaysEnabledLogSink) WithCallDepth(depth int) logr.LogSink {
	callDepthLogSink, ok := l.LogSink.(logr.CallDepthLogSink)
	if ok {
		return &alwaysEnabledLogSink{
			LogSink: callDepthLogSink.WithCallDepth(depth),
		}
	}

	return l
}

// Override Enabled to always return true since we rely on klog itself to do log
// level filtering.
func (l *alwaysEnabledLogSink) Enabled(_ int) bool {
	return true
}
