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

package grpc

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/sirupsen/logrus"
)

// Resource represents a source of proto.Messages that can be registered
// for interest.
type Resource interface {
	// Contents returns the contents of this resource.
	Contents() []proto.Message

	// Query returns an entry for each resource name supplied.
	Query(names []string) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(chan int, int)

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string
}

// xdsHandler implements the Envoy xDS gRPC protocol.
type xdsHandler struct {
	logrus.FieldLogger
	connections counter
	resources   map[string]Resource // registered resource types
}

type grpcStream interface {
	Context() context.Context
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
}

// stream processes a stream of DiscoveryRequests.
func (xh *xdsHandler) stream(st grpcStream) (err error) {
	// bump connection counter and set it as a field on the logger
	log := xh.WithField("connection", xh.connections.next())

	// set up some nice function exit handling which notifies if the
	// stream terminated on error or not.
	defer func() {
		if err != nil {
			log.WithError(err).Error("stream terminated")
		} else {
			log.Info("stream terminated")
		}
	}()

	ch := make(chan int, 1)

	// internally all registration values start at zero so sending
	// a last that is less than zero will guarantee that each stream
	// will generate a response immediately, then wait.
	last := -1
	ctx := st.Context()

	// now stick in this loop until the client disconnects.
	for {
		// first we wait for the request from Envoy, this is part of
		// the xDS protocol.
		req, err := st.Recv()
		if err != nil {
			return err
		}

		// TODO(dfc) issue 1176: handle xDS ACK/NACK

		// from the request we derive the resource to stream which have
		// been registered according to the typeURL.
		r, ok := xh.resources[req.TypeUrl]
		if !ok {
			return fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
		}

		// stick some debugging details on the logger, not that we redeclare log in this scope
		// so the next time around the loop all is forgotten.
		log := log.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail)

		log.Info("stream_wait")

		// now we wait for a notification, if this is the first request received on this
		// connection last will be less than zero and that will trigger a response immediately.
		r.Register(ch, last)
		select {
		case last = <-ch:
			// boom, something in the cache has changed.
			// TODO(dfc) the thing that has changed may not be in the scope of the filter
			// so we're going to be sending an update that is a no-op. See #426

			var resources []proto.Message
			switch len(req.ResourceNames) {
			case 0:
				// no resource hints supplied, return the full
				// contents of the resource
				resources = r.Contents()
			default:
				// resource hints supplied, return exactly those
				resources = r.Query(req.ResourceNames)
			}

			any, err := toAny(r.TypeURL(), resources)
			if err != nil {
				return err
			}

			resp := &v2.DiscoveryResponse{
				VersionInfo: strconv.Itoa(last),
				Resources:   any,
				TypeUrl:     r.TypeURL(),
				Nonce:       strconv.Itoa(last),
			}
			if err := st.Send(resp); err != nil {
				return err
			}
			log.WithField("count", len(resources)).Info("response")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// toAny converts the contents of a resourcer's Values to the
// respective slice of types.Any.
func toAny(typeURL string, values []proto.Message) ([]types.Any, error) {
	var resources []types.Any
	for _, value := range values {
		v, err := proto.Marshal(value)
		if err != nil {
			return nil, err
		}
		resources = append(resources, types.Any{TypeUrl: typeURL, Value: v})
	}
	return resources, nil
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
