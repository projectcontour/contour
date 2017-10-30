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

// workgroup provides a mechanism for controlling the lifetime
// of a group of related goroutines (workers).
// workgroup is distilled from similar ideas in Peter Bourgon's
// github.com/oklog/oklog/pkg/group and Dave Cheney's github.com/pkg/life.
package workgroup

// TODO(dfc) propogation of errors is compeltely absent at the moment.
// The main reason for this is the IndexerInformer does not return an error
// so there is nothing to propogate. This isn't excellent, and will be addressed
// at some point, but for the moment there is nothing to return because nothing
// is returned to the Group.

import "sync"

// Group manages a set of goroutines with related lifetimes.
type Group struct {
	fn []func(<-chan struct{})
}

// Add adds a function to the Group.
// The function will be exectuted in its own goroutine when Run is called.
// Add must be called before Run.
func (g *Group) Add(fn func(<-chan struct{})) {
	g.fn = append(g.fn, fn)
}

// Run exectues each function registered with Add in its own goroutine.
// Run blocks until each function has returned.
// The first function to return will trigger the closure of the channel
// passed to each function, who should in turn, return.
func (g *Group) Run() {
	var wg sync.WaitGroup
	wg.Add(len(g.fn))

	stop := make(chan struct{})
	result := make(chan error, len(g.fn))
	for _, fn := range g.fn {
		go func(fn func(<-chan struct{})) {
			defer wg.Done()
			fn(stop)
			result <- nil
		}(fn)
	}

	<-result    // wait for first goroutine to exit
	close(stop) // ask others to exit
	wg.Wait()   // wait for all goroutines to exit
}
