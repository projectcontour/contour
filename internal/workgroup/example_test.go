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

package workgroup_test

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/projectcontour/contour/internal/workgroup"
)

func ExampleGroup_Run() {
	var g workgroup.Group

	a := func(stop <-chan struct{}) error {
		defer fmt.Println("A stopped")
		<-time.After(100 * time.Millisecond)
		return fmt.Errorf("timed out")
	}
	g.Add(a)

	b := func(stop <-chan struct{}) error {
		defer fmt.Println("B stopped")
		<-stop
		return nil
	}
	g.Add(b)

	err := g.Run()
	fmt.Println(err)

	// Output:
	// A stopped
	// B stopped
	// timed out
}

func ExampleGroup_Run_withShutdown() {
	var g workgroup.Group

	shutdown := make(chan time.Time)
	g.Add(func(<-chan struct{}) error {
		<-shutdown
		return fmt.Errorf("shutdown")
	})

	g.Add(func(stop <-chan struct{}) error {
		<-stop
		return fmt.Errorf("terminated")
	})

	go func() {
		shutdown <- <-time.After(100 * time.Millisecond)
	}()

	err := g.Run()
	fmt.Println(err)

	// Output:
	// shutdown
}

func ExampleGroup_Run_multipleListeners() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "Hello, HTTP!")
	})

	var g workgroup.Group

	// listen on port 80
	g.Add(func(stop <-chan struct{}) error {
		l, err := net.Listen("tcp", ":80") // nolint:gosec
		if err != nil {
			return err
		}

		go func() {
			<-stop
			l.Close()
		}()
		return http.Serve(l, mux)
	})

	// listen on port 443
	g.Add(func(stop <-chan struct{}) error {
		l, err := net.Listen("tcp", ":443") // nolint:gosec
		if err != nil {
			return err
		}

		go func() {
			<-stop
			l.Close()
		}()
		return http.Serve(l, mux)
	})

	g.Run() // nolint:errcheck
}
