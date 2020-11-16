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

package fixture

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

// NewTestLogger returns logrus.Logger that writes messages using (*testing.T)Logf.
func NewTestLogger(t *testing.T) *logrus.Logger {
	log := logrus.New()
	log.Out = &testWriter{t}
	return log
}

type discardWriter struct{}

func (d *discardWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}

// NewDiscardLogger returns logrus.Logger that discards log messages.
func NewDiscardLogger() *logrus.Logger {
	log := logrus.New()
	log.Out = &discardWriter{}
	return log
}
