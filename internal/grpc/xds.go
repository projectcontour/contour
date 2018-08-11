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
	"sync/atomic"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/sirupsen/logrus"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

// xdsHandler implements the Envoy xDS gRPC protocol.
type xdsHandler struct {
	logrus.FieldLogger
	connections counter
	resources   map[string]resource // registered resource types
}

// fetch handles a single DiscoveryRequest.
func (xh *xdsHandler) fetch(req *v2.DiscoveryRequest) (*v2.DiscoveryResponse, error) {
	xh.WithField("connection", xh.connections.next()).WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail).Info("fetch")
	r, ok := xh.resources[req.TypeUrl]
	if !ok {
		return nil, fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
	}
	resources, err := toAny(r, toFilter(req.ResourceNames))
	return &v2.DiscoveryResponse{
		VersionInfo: "0",
		Resources:   resources,
		TypeUrl:     r.TypeURL(),
		Nonce:       "0",
	}, err
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

		// from the request we derive the resource to stream which have
		// been registered according to the typeURL.
		r, ok := xh.resources[req.TypeUrl]
		if !ok {
			return fmt.Errorf("no resource registered for typeURL %q", req.TypeUrl)
		}

		// stick some debugging details on the logger, not that we redeclare log in this scope
		// so the next time around the loop all is forgotten.
		log := log.WithField("version_info", req.VersionInfo).WithField("resource_names", req.ResourceNames).WithField("type_url", req.TypeUrl).WithField("response_nonce", req.ResponseNonce).WithField("error_detail", req.ErrorDetail)

		for {
			log.Info("stream_wait")

			// now we wait for a notification, if this is the first time through the loop
			// then last will be zero and that will trigger a notification immediately.
			r.Register(ch, last)
			select {
			case last = <-ch:
				// boom, something in the cache has changed.
				// TODO(dfc) the thing that has changed may not be in the scope of the filter
				// so we're going to be sending an update that is a no-op. See #426

				// generate a filter from the request, then call toAny which
				// will get r's (our resource) filter values, then convert them
				// to the types.Any from required by gRPC.
				resources, err := toAny(r, toFilter(req.ResourceNames))
				if err != nil {
					return err
				}

				resp := &v2.DiscoveryResponse{
					VersionInfo: "0",
					Resources:   resources,
					TypeUrl:     r.TypeURL(),
					Nonce:       "0",
				}
				if err := st.Send(resp); err != nil {
					return err
				}
				log.WithField("count", len(resources)).Info("response")

				// ok, the client hung up, return any error stored in the context and we're done.
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// toAny converts the contents of a resourcer's Values to the
// respective slice of types.Any.
func toAny(res resource, filter func(string) bool) ([]types.Any, error) {
	v := res.Values(filter)
	resources := make([]types.Any, len(v))
	for i := range v {
		value, err := proto.Marshal(v[i])
		if err != nil {
			return nil, err
		}
		resources[i] = types.Any{TypeUrl: res.TypeURL(), Value: value}
	}
	return resources, nil
}

// toFilter converts a slice of strings into a filter function.
// If the slice is empty, then a filter function that matches everything
// is returned.
func toFilter(names []string) func(string) bool {
	if len(names) == 0 {
		return func(string) bool { return true }
	}
	m := make(map[string]bool)
	for _, n := range names {
		m[n] = true
	}
	return func(name string) bool { return m[name] }
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
