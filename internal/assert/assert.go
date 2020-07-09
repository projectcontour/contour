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

// Package assert provides assertion helpers
package assert

import (
	"testing"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type Assert struct {
	t *testing.T
}

// Equal will test that want == got, and call t.Fatal if it does not.
// Notably, for errors, they are equal if they are both nil, or are both non-nil.
// No value information is checked for errors.
func Equal(t *testing.T, want, got interface{}) {
	t.Helper()
	Assert{t}.Equal(want, got)
}

// Equal will call t.Fatal if want and got are not equal.
func (a Assert) Equal(want, got interface{}) {
	a.t.Helper()
	opts := []cmp.Option{
		cmpopts.IgnoreFields(v2.DiscoveryResponse{}, "VersionInfo", "Nonce"),
		cmpopts.AcyclicTransformer("UnmarshalAny", unmarshalAny),
		// errors to be equal only if both are nil or both are non-nil.
		cmp.Comparer(func(x, y error) bool {
			return (x == nil) == (y == nil)
		}),
	}
	diff := cmp.Diff(want, got, opts...)
	if diff != "" {
		a.t.Fatal(diff)
	}
}

func unmarshalAny(a *any.Any) proto.Message {
	if a == nil {
		return nil
	}
	pb, err := ptypes.Empty(a)
	if err != nil {
		panic(err.Error())
	}
	err = ptypes.UnmarshalAny(a, pb)
	if err != nil {
		panic(err.Error())
	}
	return pb
}
