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

	"github.com/bombsimon/logrusr/v2"
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

		// Use the LogSink from a logrusr Logger, but wrapped in
		// an adapter that always returns true for Enabled() since
		// we rely on klog to do log level filtering.
		klog.SetLogger(logr.New(&alwaysEnabledLogSink{
			LogSink: logrusr.New(p.log, logrusr.WithReportCaller()).GetSink(),
		}))
	}
}

type alwaysEnabledLogSink struct {
	logr.LogSink
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
func (l *alwaysEnabledLogSink) Enabled(level int) bool {
	return true
}
