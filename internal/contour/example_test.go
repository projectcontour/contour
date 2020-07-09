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

package contour_test

import (
	"context"
	"fmt"
	"time"

	"github.com/projectcontour/contour/internal/contour"
)

func ExampleCond() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	ch := make(chan int, 1)
	last := 0
	var c contour.Cond
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			c.Notify()
		}
	}()

	for {
		c.Register(ch, last)
		select {
		case last = <-ch:
			fmt.Println("notification received:", last)
		case <-ctx.Done():
			fmt.Println("timeout")
			return
		}
	}
}
