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

// Package log provides an interface for a generic logging solution for Go projects.
package log

// A Logger represents the ability to log informational and error messages.
type Logger interface {
	// All Loggers implement InfoLogger.  Calling InfoLogger methods directly on
	// a Logger value is equivalent to calling them on a V(1) InfoLogger.  For
	// example, logger.Info() produces the same result as logger.V(1).Info.
	InfoLogger

	// Error logs an error message.  This is behaviorally akin to fmt.Println.
	Error(args ...interface{})

	// Errorf logs a formatted error message.
	Errorf(format string, args ...interface{})

	// V returns an InfoLogger value for a specific verbosity level. A higher
	// verbosity level means a log message is less important.
	V(level int) InfoLogger

	// NewWithPrefix returns a Logger which prefixes all messages.
	WithPrefix(prefix string) Logger
}

// An InfoLogger represents the ability to log informational messages.
type InfoLogger interface {
	// Infof logs a formatted non-error message.
	Infof(format string, args ...interface{})
}
