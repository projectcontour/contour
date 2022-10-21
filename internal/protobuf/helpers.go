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
	"fmt"
	"reflect"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Duration converts a time.Duration to a pointer to a duration.Duration.
func Duration(d time.Duration) *durationpb.Duration {
	return &durationpb.Duration{
		Seconds: int64(d / time.Second),
		Nanos:   int32(d % time.Second),
	}
}

// UInt32 converts a uint32 to a pointer to a wrappers.UInt32Value.
func UInt32(val uint32) *wrapperspb.UInt32Value {
	return &wrapperspb.UInt32Value{
		Value: val,
	}
}

// UInt32OrDefault returns a wrapped UInt32Value. If val is 0, def is wrapped and returned.
func UInt32OrDefault(val uint32, def uint32) *wrapperspb.UInt32Value {
	switch val {
	case 0:
		return UInt32(def)
	default:
		return UInt32(val)
	}
}

// UInt32OrNil returns a wrapped UInt32Value. If val is 0, nil is returned
func UInt32OrNil(val uint32) *wrapperspb.UInt32Value {
	switch val {
	case 0:
		return nil
	default:
		return UInt32(val)
	}
}

// Bool converts a bool to a pointer to a wrappers.BoolValue.
func Bool(val bool) *wrapperspb.BoolValue {
	return &wrapperspb.BoolValue{
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

// MustMarshalAny marshals a protobuf into an any.Any type, panicking
// if that operation fails.
func MustMarshalAny(pb proto.Message) *anypb.Any {
	a, err := anypb.New(pb)
	if err != nil {
		panic(err.Error())
	}

	return a
}

// AnyMessageTypeOf returns the any.Any type of msg.
func AnyMessageTypeOf(msg proto.Message) string {
	a := MustMarshalAny(msg)
	return a.TypeUrl
}

// MustMarshalJSON marshals msg to indented JSON.
func MustMarshalJSON(msg proto.Message) string {
	m := protojson.MarshalOptions{Indent: "  "}

	res, err := m.Marshal(msg)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %T: %s", msg, err))
	}

	return string(res)
}
