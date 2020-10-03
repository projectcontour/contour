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
	"io/ioutil"
	"os"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
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
			kingpin.Fatalf("you must supply all three TLS parameters - --cafile, --cert-file, --key-file, or none of them")
		}
		// Load the client certificates from disk
		certificate, err := tls.LoadX509KeyPair(c.ClientCert, c.ClientKey)
		kingpin.FatalIfError(err, "failed to load certificates from disk")
		// Create a certificate pool from the certificate authority
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(c.CAFile)
		kingpin.FatalIfError(err, "failed to read CA cert")

		// Append the certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			// TODO(nyoung) OMG yuck, thanks for this, crypto/tls. Suggestions on alternates welcomed.
			kingpin.Fatalf("failed to append CA certs")
		}

		creds := credentials.NewTLS(&tls.Config{
			// TODO(youngnick): Does this need to be defaulted with a cli flag to
			// override?
			// The ServerName here needs to be one of the SANs available in
			// the serving cert used by contour serve.
			ServerName:   "contour",
			Certificates: []tls.Certificate{certificate},
			RootCAs:      certPool,
			MinVersion:   tls.VersionTLS12,
		})
		options = append(options, grpc.WithTransportCredentials(creds))
	default:
		options = append(options, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(c.ContourAddr, options...)
	kingpin.FatalIfError(err, "failed connecting Contour Server")

	return conn
}

// ClusterStream returns a stream of Clusters using the config in the Client.
func (c *Client) ClusterStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewClusterDiscoveryServiceClient(c.dial()).StreamClusters(context.Background())
	kingpin.FatalIfError(err, "failed to fetch stream of Clusters")
	return stream
}

// EndpointStream returns a stream of Endpoints using the config in the Client.
func (c *Client) EndpointStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewEndpointDiscoveryServiceClient(c.dial()).StreamEndpoints(context.Background())
	kingpin.FatalIfError(err, "failed to fetch stream of Endpoints")
	return stream
}

// ListenerStream returns a stream of Listeners using the config in the Client.
func (c *Client) ListenerStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewListenerDiscoveryServiceClient(c.dial()).StreamListeners(context.Background())
	kingpin.FatalIfError(err, "failed to fetch stream of Listeners")
	return stream
}

// RouteStream returns a stream of Routes using the config in the Client.
func (c *Client) RouteStream() v2.ClusterDiscoveryService_StreamClustersClient {
	stream, err := v2.NewRouteDiscoveryServiceClient(c.dial()).StreamRoutes(context.Background())
	kingpin.FatalIfError(err, "failed to fetch stream of Routes")
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
		kingpin.FatalIfError(err, "failed to send Discover Request")
		resp, err := st.Recv()
		kingpin.FatalIfError(err, "failed to receive response for Discover Request")
		err = m.Marshal(os.Stdout, resp)
		kingpin.FatalIfError(err, "failed to marshal Discovery Response")
	}
}
