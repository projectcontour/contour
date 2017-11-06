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

package stdlog

import (
	"bytes"
	"os"
	"testing"

	"github.com/heptio/contour/internal/log"
)

func TestNew(t *testing.T) {
	var _ log.Logger = New(os.Stdout, os.Stderr, 0)
}

func TestV(t *testing.T) {
	var root log.Logger = New(os.Stdout, os.Stderr, 0)
	var _ log.InfoLogger = root.V(99)
}

func TestInfof(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	root.Infof("stdout %q", "%")

	const want = "stdout \"%\"\n"
	if stdout.String() != want {
		t.Fatalf("expected %q, got %q", want, stdout.String())
	}

	if stderr.String() != "" {
		t.Fatalf("expected %q, got %q", "", stderr.String())
	}
}

func TestError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	root.Error("stderr %q", "%")

	if stdout.String() != "" {
		t.Fatalf("expected %q, got %q", "", stdout.String())
	}

	const want = "stderr %q %\n"
	if stderr.String() != want {
		t.Fatalf("expected %q, got %q", want, stderr.String())
	}
}

func TestErrorf(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	root.Errorf("stderr %q", "%")

	if stdout.String() != "" {
		t.Fatalf("expected %q, got %q", "", stdout.String())
	}

	const want = "stderr \"%\"\n"
	if stderr.String() != want {
		t.Fatalf("expected %q, got %q", want, stderr.String())
	}
}

func TestPrefixInfof(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	var log = root.WithPrefix("prefix")
	log.Infof("stdout %q", "%")

	const want = "prefix: stdout \"%\"\n"
	if stdout.String() != want {
		t.Fatalf("expected %q, got %q", want, stdout.String())
	}

	if stderr.String() != "" {
		t.Fatalf("expected %q, got %q", "", stderr.String())
	}
}

func TestPrefixError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	var log = root.WithPrefix("prefix")
	log.Error("stderr %q", "%")

	if stdout.String() != "" {
		t.Fatalf("expected %q, got %q", "", stdout.String())
	}

	const want = "prefix: stderr %q %\n"
	if stderr.String() != want {
		t.Fatalf("expected %q, got %q", want, stderr.String())
	}
}

func TestPrefixErrorf(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const NOFLAGS = 1 << 16
	var root log.Logger = New(&stdout, &stderr, NOFLAGS)
	var log = root.WithPrefix("prefix")
	log.Errorf("stderr %q", "%")

	if stdout.String() != "" {
		t.Fatalf("expected %q, got %q", "", stdout.String())
	}

	const want = "prefix: stderr \"%\"\n"
	if stderr.String() != want {
		t.Fatalf("expected %q, got %q", want, stderr.String())
	}
}
