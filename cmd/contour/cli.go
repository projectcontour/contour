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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"log"
	"os"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client holds the details for the cli client to connect to.
type Client struct {
	ContourAddr string
	CAFile      string
	ClientCert  string
	ClientKey   string
}

func (c *Client) dial() *grpc.ClientConn {

	var options []grpc.DialOption

	// Check the TLS setup
	switch {
	case c.CAFile != "" || c.ClientCert != "" || c.ClientKey != "":
		// If one of the three TLS commands is not empty, they all must be not empty
		if !(c.CAFile != "" && c.ClientCert != "" && c.ClientKey != "") {
			log.Fatal("You must supply all three TLS parameters - --cafile, --cert-file, --key-file, or none of them.")
		}
		// Load the client certificates from disk
		certificate, err := tls.LoadX509KeyPair(c.ClientCert, c.ClientKey)
		check(err)

		// Create a certificate pool from the certificate authority
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(c.CAFile)
		check(err)

		// Append the certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			// TODO(nyoung) OMG yuck, thanks for this, crypto/tls. Suggestions on alternates welcomed.
			check(errors.New("failed to append ca certs"))
		}

		creds := credentials.NewTLS(&tls.Config{
			// TODO(youngnick): Does this need to be defaulted with a cli flag to
			// override?
			// The ServerName here needs to be one of the SANs available in
			// the serving cert used by contour serve.
			ServerName:   "contour",
			Certificates: []tls.Certificate{certificate},
			RootCAs:      certPool,
		})
		options = append(options, grpc.WithTransportCredentials(creds))
	default:
		options = append(options, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(c.ContourAddr, options...)
	check(err)

	return conn
}

// ClusterStream returns a stream of Clusters using the config in the Client.
func (c *Client) ClusterStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewClusterDiscoveryServiceClient(c.dial()).StreamClusters(context.Background())
	check(err)
	return stream
}

// EndpointStream returns a stream of Endpoints using the config in the Client.
func (c *Client) EndpointStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewEndpointDiscoveryServiceClient(c.dial()).StreamEndpoints(context.Background())
	check(err)
	return stream
}

// ListenerStream returns a stream of Listeners using the config in the Client.
func (c *Client) ListenerStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewListenerDiscoveryServiceClient(c.dial()).StreamListeners(context.Background())
	check(err)
	return stream
}

// RouteStream returns a stream of Routes using the config in the Client.
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
