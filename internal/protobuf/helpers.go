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
	"reflect"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// Duration converts a time.Duration to a pointer to a duration.Duration.
func Duration(d time.Duration) *duration.Duration {
	return &duration.Duration{
		Seconds: int64(d / time.Second),
		Nanos:   int32(d % time.Second),
	}
}

// UInt32 converts a uint32 to a pointer to a wrappers.UInt32Value.
func UInt32(val uint32) *wrappers.UInt32Value {
	return &wrappers.UInt32Value{
		Value: val,
	}
}

// Bool converts a bool to a pointer to a wrappers.BoolValue.
func Bool(val bool) *wrappers.BoolValue {
	return &wrappers.BoolValue{
		Value: val,
	}
}

// AsMessages casts the given slice of values (that implement the proto.Message
// interface) to a slice of proto.Message. If the length of the slice is 0, it
// returns nil.
func AsMessages(messages interface{}) []proto.Message {
	v := reflect.ValueOf(messages)
	if v.Len() == 0 {
		return nil
	}

	protos := make([]proto.Message, v.Len())

	for i := range protos {
		protos[i] = v.Index(i).Interface().(proto.Message)
	}

	return protos
}

// MustMarshalAny marshals a protobug into an any.Any type, panicing
// if that operation fails.
func MustMarshalAny(pb proto.Message) *any.Any {
	a, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}

	return a
}
