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
	"context"
	"os"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
)

type Client struct {
	ContourAddr string
}

func (c *Client) dial() *grpc.ClientConn {
	conn, err := grpc.Dial(c.ContourAddr, grpc.WithInsecure())
	check(err)
	return conn
}

func (c *Client) ClusterStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewClusterDiscoveryServiceClient(c.dial()).StreamClusters(context.Background())
	check(err)
	return stream
}

func (c *Client) EndpointStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewEndpointDiscoveryServiceClient(c.dial()).StreamEndpoints(context.Background())
	check(err)
	return stream
}

func (c *Client) ListenerStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewListenerDiscoveryServiceClient(c.dial()).StreamListeners(context.Background())
	check(err)
	return stream
}

func (c *Client) RouteStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewRouteDiscoveryServiceClient(c.dial()).StreamRoutes(context.Background())
	check(err)
	return stream
}

type stream interface {
	Send(*v2.DiscoveryRequest) error
	Recv() (*v2.DiscoveryResponse, error)
}

func watchstream(st stream, typeURL string, resources []string) {
	m := proto.TextMarshaler{
		Compact:   false,
		ExpandAny: true,
	}
	for {
		req := &v2.DiscoveryRequest{
			TypeUrl:       typeURL,
			ResourceNames: resources,
		}
		err := st.Send(req)
		check(err)
		resp, err := st.Recv()
		check(err)
		err = m.Marshal(os.Stdout, resp)
		check(err)
	}
}
