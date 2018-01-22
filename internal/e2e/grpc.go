// Copyright © 2018 Heptio
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

package e2e

// grpc helpers

import (
	"net"
	"sync"
	"testing"

	"github.com/heptio/contour/internal/contour"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/log/stdlog"
	"google.golang.org/grpc"
	"k8s.io/client-go/tools/cache"
)

type testLogger struct {
	*testing.T
}

func (t *testLogger) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

func setup(t *testing.T) (cache.ResourceEventHandler, *grpc.ClientConn, func()) {
	w := &testLogger{t}

	log := stdlog.New(w, w, 0)

	tr := &contour.Translator{
		Logger: log,
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	check(t, err)
	var wg sync.WaitGroup
	wg.Add(1)
	srv := cgrpc.NewAPI(log, tr)
	go func() {
		defer wg.Done()
		srv.Serve(l)
	}()
	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	check(t, err)
	return tr, cc, func() {
		l.Close()
		srv.Stop()
		wg.Wait()
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
