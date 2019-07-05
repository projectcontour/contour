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

package main

import (
	"reflect"
	"testing"
)

func TestServeContextIngressRouteRootNamespaces(t *testing.T) {
	tests := map[string]struct {
		ctx  serveContext
		want []string
	}{
		"empty": {
			ctx: serveContext{
				rootNamespaces: "",
			},
			want: nil,
		},
		"blank-ish": {
			ctx: serveContext{
				rootNamespaces: " \t ",
			},
			want: nil,
		},
		"one value": {
			ctx: serveContext{
				rootNamespaces: "heptio-contour",
			},
			want: []string{"heptio-contour"},
		},
		"multiple, easy": {
			ctx: serveContext{
				rootNamespaces: "prod1,prod2,prod3",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
		"multiple, hard": {
			ctx: serveContext{
				rootNamespaces: "prod1, prod2, prod3 ",
			},
			want: []string{"prod1", "prod2", "prod3"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := tc.ctx.ingressRouteRootNamespaces()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected: %q, got: %q", tc.want, got)
			}
		})
	}
}
