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

// Package protobuf provides helpers for working with golang/protobuf types.
package protobuf

import (
	"math"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// UInt32OrDefault returns a wrapped UInt32Value. If val is 0, def is wrapped and returned.
func UInt32OrDefault(val, def uint32) *wrapperspb.UInt32Value {
	switch val {
	case 0:
		return wrapperspb.UInt32(def)
	default:
		return wrapperspb.UInt32(val)
	}
}

// UInt32OrNil returns a wrapped UInt32Value. If val is 0, nil is returned
func UInt32OrNil(val uint32) *wrapperspb.UInt32Value {
	switch val {
	case 0:
		return nil
	default:
		return wrapperspb.UInt32(val)
	}
}

// SafeIntToUint32 converts an int to uint32 with bounds checking.
// Returns 0 if the value is negative or exceeds uint32 max.
func SafeIntToUint32(val int) uint32 {
	if val < 0 || val > math.MaxUint32 {
		return 0
	}
	return uint32(val)
}

// AsMessages converts the given slice of values (that implement the proto.Message
// interface) to a slice of proto.Message. If the length of the slice is 0, it
// returns nil.
func AsMessages[T proto.Message](messages []T) []proto.Message {
	if len(messages) == 0 {
		return nil
	}

	protos := make([]proto.Message, len(messages))
	for i, message := range messages {
		protos[i] = message
	}
	return protos
}

// MustMarshalAny marshals a protobuf into an any.Any type, panicking
// if that operation fails.
func MustMarshalAny(pb proto.Message) *anypb.Any {
	a, err := anypb.New(pb)
	if err != nil {
		panic(err.Error())
	}

	return a
}
