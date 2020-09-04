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

package contour

// Observer is an interface for receiving notifications.
type Observer interface {
	Refresh()
}

// ObserverFunc is a function that implements the Observer interface
// by calling itself. It can be nil.
type ObserverFunc func()

func (f ObserverFunc) Refresh() {
	if f != nil {
		f()
	}
}

var _ Observer = ObserverFunc(nil)

// ComposeObservers returns a new Observer that calls each of its arguments in turn.
func ComposeObservers(observers ...Observer) Observer {
	return ObserverFunc(func() {
		for _, o := range observers {
			o.Refresh()
		}
	})
}
