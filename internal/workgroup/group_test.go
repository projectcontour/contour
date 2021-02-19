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

package workgroup

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupRunWithNoRegisteredFunctions(t *testing.T) {
	var g Group
	got := g.Run(context.TODO())
	assert(t, nil, got)
}

func TestGroupFirstReturnValueIsReturnedToRunsCaller(t *testing.T) {
	var g Group
	wait := make(chan int)
	g.Add(func(<-chan struct{}) error {
		<-wait
		return io.EOF
	})

	g.Add(func(stop <-chan struct{}) error {
		<-stop
		return errors.New("stopped")
	})

	result := make(chan error)
	go func() {
		result <- g.Run(context.TODO())
	}()
	close(wait)
	assert(t, io.EOF, <-result)
}

func TestGroupAddContext(t *testing.T) {
	var g Group
	wait := make(chan int)
	g.Add(func(<-chan struct{}) error {
		<-wait
		return io.EOF
	})

	g.AddContext(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	result := make(chan error)
	go func() {
		result <- g.Run(context.TODO())
	}()
	close(wait)
	assert(t, io.EOF, <-result)
}

func TestGroupCancellation(t *testing.T) {
	var g Group
	ctx, cancel := context.WithCancel(context.Background())

	const tasks = 100
	var count int32

	for i := 0; i < tasks; i++ {
		g.Add(func(stop <-chan struct{}) error {
			defer atomic.AddInt32(&count, 1)
			defer time.Sleep(time.Millisecond * time.Duration(i))
			<-stop
			return nil
		})
	}

	done := make(chan error)
	go func() {
		done <- g.Run(ctx)
	}()

	cancel()
	<-done

	if got := atomic.LoadInt32(&count); got != tasks {
		t.Errorf("expected: %d, got: %d", tasks, got)
	}
}

func assert(t *testing.T, want, got error) {
	t.Helper()
	if want != got {
		t.Fatalf("expected: %v, got: %v", want, got)
	}
}
