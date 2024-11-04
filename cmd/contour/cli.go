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
	"fmt"
	"os"

	"github.com/alecthomas/kingpin/v2"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	"github.com/sirupsen/logrus"
	grpc_code "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

// registerCli registers the cli subcommand and flags
// with the Application provided.
func registerCli(app *kingpin.Application, log *logrus.Logger) (*kingpin.CmdClause, *Client) {
	client := Client{Log: log}

	cli := app.Command("cli", "A CLI client for the Contour Kubernetes ingress controller.")
	cli.Flag("cafile", "CA bundle file for connecting to a TLS-secured Contour.").Envar("CLI_CAFILE").StringVar(&client.CAFile)
	cli.Flag("cert-file", "Client certificate file for connecting to a TLS-secured Contour.").Envar("CLI_CERT_FILE").StringVar(&client.ClientCert)
	cli.Flag("contour", "Contour host:port.").Default("127.0.0.1:8001").StringVar(&client.ContourAddr)
	cli.Flag("delta", "Use incremental xDS.").BoolVar(&client.Delta)
	cli.Flag("key-file", "Client key file for connecting to a TLS-secured Contour.").Envar("CLI_KEY_FILE").StringVar(&client.ClientKey)
	cli.Flag("nack", "NACK all responses (for testing).").BoolVar(&client.Nack)
	cli.Flag("node-id", "Node ID for the CLI client to use.").Envar("CLI_NODE_ID").Default("ContourCLI").StringVar(&client.NodeID)

	return cli, &client
}

// Client holds the details for the cli client to connect to.
// TODO(youngnick): Move NACK handling to a sentinel, either file or keystroke.
type Client struct {
	ContourAddr string
	CAFile      string
	ClientCert  string
	ClientKey   string
	Nack        bool
	Delta       bool
	NodeID      string
	Log         *logrus.Logger
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
		if err != nil {
			c.Log.WithError(err).Fatal("failed to load certificates from disk")
		}
		// Create a certificate pool from the certificate authority
		certPool := x509.NewCertPool()
		ca, err := os.ReadFile(c.CAFile)
		if err != nil {
			c.Log.WithError(err).Fatal("failed to read CA cert")
		}

		// Append the certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			// TODO(nyoung) OMG yuck, thanks for this, crypto/tls. Suggestions on alternates welcomed.
			c.Log.Fatal("failed to append CA certs")
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
		options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(c.ContourAddr, options...)
	if err != nil {
		c.Log.WithError(err).Fatal("failed connecting Contour Server")
	}

	return conn
}

// ClusterStream returns a stream of Clusters using the config in the Client.
func (c *Client) ClusterStream() envoy_service_cluster_v3.ClusterDiscoveryService_StreamClustersClient {
	stream, err := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(c.dial()).StreamClusters(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch stream of Clusters")
	}
	return stream
}

// EndpointStream returns a stream of Endpoints using the config in the Client.
func (c *Client) EndpointStream() envoy_service_endpoint_v3.EndpointDiscoveryService_StreamEndpointsClient {
	stream, err := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(c.dial()).StreamEndpoints(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch stream of Endpoints")
	}
	return stream
}

// ListenerStream returns a stream of Listeners using the config in the Client.
func (c *Client) ListenerStream() envoy_service_listener_v3.ListenerDiscoveryService_StreamListenersClient {
	stream, err := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(c.dial()).StreamListeners(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch stream of Listeners")
	}
	return stream
}

// RouteStream returns a stream of Routes using the config in the Client.
func (c *Client) RouteStream() envoy_service_route_v3.RouteDiscoveryService_StreamRoutesClient {
	stream, err := envoy_service_route_v3.NewRouteDiscoveryServiceClient(c.dial()).StreamRoutes(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch stream of Routes")
	}
	return stream
}

type stream interface {
	Send(*envoy_service_discovery_v3.DiscoveryRequest) error
	Recv() (*envoy_service_discovery_v3.DiscoveryResponse, error)
}

func watchstream(log *logrus.Logger, st stream, typeURL string, resources []string, nack bool, nodeID string) {
	m := protojson.MarshalOptions{
		Multiline:     true,
		Indent:        "  ",
		UseProtoNames: true,
	}

	currentVersion := "0"

	// Send the initial, non-ACK discovery request.
	req := &envoy_service_discovery_v3.DiscoveryRequest{
		TypeUrl:       typeURL,
		ResourceNames: resources,
		VersionInfo:   currentVersion,
		Node: &envoy_config_core_v3.Node{
			Id: nodeID,
		},
	}
	log.WithField("currentVersion", currentVersion).Info("Sending discover request")
	fmt.Println(m.Format(req))
	err := st.Send(req)
	if err != nil {
		log.WithError(err).Fatal("failed to send Discover Request")
	}

	for {

		// Wait until we receive a response to our request.
		resp, err := st.Recv()
		if err != nil {
			log.WithError(err).Fatal("failed to receive response for Discover Request")
		}
		log.WithField("currentVersion", currentVersion).
			WithField("resp_version_info", resp.VersionInfo).
			WithField("nonce", resp.Nonce).
			Info("Received Discovery Response")

		fmt.Println(m.Format(resp))
		if err != nil {
			log.WithError(err).Fatal("failed to marshal Discovery Response")
		}

		currentVersion = resp.VersionInfo

		if nack {
			// We'll NACK the response we just got.
			// The ResponseNonce field is what makes it an ACK,
			// and the VersionInfo field must match the one in the response we
			// just got, or else the watch won't happen properly.
			// The ErrorDetail field being populated is what makes this a NACK
			// instead of an ACK.
			nackReq := &envoy_service_discovery_v3.DiscoveryRequest{
				TypeUrl:       typeURL,
				ResponseNonce: resp.Nonce,
				VersionInfo:   resp.VersionInfo,
				ErrorDetail: &status.Status{
					Code:    int32(grpc_code.Code_INTERNAL),
					Message: "Told to create a NACK for testing",
				},
				Node: &envoy_config_core_v3.Node{
					Id: nodeID,
				},
			}
			log.WithField("response_nonce", resp.Nonce).
				WithField("version_info", resp.VersionInfo).
				WithField("currentVersion", currentVersion).
				Info("Sending NACK discover request")

			fmt.Println(m.Format(nackReq))
			err := st.Send(nackReq)
			if err != nil {
				log.WithError(err).Fatal("failed to send NACK Discover Request")
			}

		} else {
			// We'll ACK our request.
			// The ResponseNonce field is what makes it an ACK,
			// and the VersionInfo field must match the one in the response we
			// just got, or else the watch won't happen properly.
			ackReq := &envoy_service_discovery_v3.DiscoveryRequest{
				TypeUrl:       typeURL,
				ResponseNonce: resp.Nonce,
				VersionInfo:   resp.VersionInfo,
				Node: &envoy_config_core_v3.Node{
					Id: nodeID,
				},
			}
			log.WithField("response_nonce", resp.Nonce).
				WithField("version_info", resp.VersionInfo).
				WithField("currentVersion", currentVersion).
				Info("Sending ACK discover request")
			fmt.Println(m.Format(ackReq))
			err := st.Send(ackReq)
			if err != nil {
				log.WithError(err).Fatal("failed to send ACK Discover Request")
			}

		}
	}
}

// ClusterStream returns a stream of Clusters using the config in the Client.
func (c *Client) DeltaClusterStream() envoy_service_cluster_v3.ClusterDiscoveryService_DeltaClustersClient {
	stream, err := envoy_service_cluster_v3.NewClusterDiscoveryServiceClient(c.dial()).DeltaClusters(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch incremental stream of Clusters")
	}
	return stream
}

// EndpointStream returns a stream of Endpoints using the config in the Client.
func (c *Client) DeltaEndpointStream() envoy_service_endpoint_v3.EndpointDiscoveryService_DeltaEndpointsClient {
	stream, err := envoy_service_endpoint_v3.NewEndpointDiscoveryServiceClient(c.dial()).DeltaEndpoints(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch incremental stream of Endpoints")
	}
	return stream
}

// ListenerStream returns a stream of Listeners using the config in the Client.
func (c *Client) DeltaListenerStream() envoy_service_listener_v3.ListenerDiscoveryService_DeltaListenersClient {
	stream, err := envoy_service_listener_v3.NewListenerDiscoveryServiceClient(c.dial()).DeltaListeners(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch incremental stream of Listeners")
	}
	return stream
}

// RouteStream returns a stream of Routes using the config in the Client.
func (c *Client) DeltaRouteStream() envoy_service_route_v3.RouteDiscoveryService_DeltaRoutesClient {
	stream, err := envoy_service_route_v3.NewRouteDiscoveryServiceClient(c.dial()).DeltaRoutes(context.Background())
	if err != nil {
		c.Log.WithError(err).Fatal("failed to fetch incremental stream of Routes")
	}
	return stream
}

type deltaStream interface {
	Send(*envoy_service_discovery_v3.DeltaDiscoveryRequest) error
	Recv() (*envoy_service_discovery_v3.DeltaDiscoveryResponse, error)
}

func watchDeltaStream(log *logrus.Logger, st deltaStream, typeURL string, resources []string, nack bool, nodeID string) {
	m := protojson.MarshalOptions{
		Multiline:     true,
		Indent:        "  ",
		UseProtoNames: true,
	}

	currentVersion := "0"

	// Send the initial, non-ACK discovery request.
	req := &envoy_service_discovery_v3.DeltaDiscoveryRequest{
		TypeUrl:                typeURL,
		ResourceNamesSubscribe: resources,
		Node: &envoy_config_core_v3.Node{
			Id: nodeID,
		},
	}
	log.WithField("currentVersion", currentVersion).Info("Sending incremental discover request")
	fmt.Println(m.Format(req))
	err := st.Send(req)
	if err != nil {
		log.WithError(err).Fatal("failed to send incremental Discover Request")
	}

	for {

		// Wait until we receive a response to our request.
		resp, err := st.Recv()
		if err != nil {
			log.WithError(err).Fatal("failed to receive response for incremental Discover Request")
		}
		log.WithField("currentVersion", currentVersion).
			WithField("resp_system_version_info", resp.SystemVersionInfo).
			WithField("nonce", resp.Nonce).
			Info("Received Discovery Response")

		fmt.Println(m.Format(resp))
		if err != nil {
			log.WithError(err).Fatal("failed to marshal incremental Discovery Response")
		}

		currentVersion = resp.SystemVersionInfo

		if nack {
			// We'll NACK the response we just got.
			// The ResponseNonce field is what makes it an ACK.
			// The ErrorDetail field being populated is what makes this a NACK
			// instead of an ACK.
			nackReq := &envoy_service_discovery_v3.DeltaDiscoveryRequest{
				TypeUrl:       typeURL,
				ResponseNonce: resp.Nonce,
				ErrorDetail: &status.Status{
					Code:    int32(grpc_code.Code_INTERNAL),
					Message: "Told to create a NACK for testing",
				},
				Node: &envoy_config_core_v3.Node{
					Id: nodeID,
				},
			}
			log.WithField("response_nonce", resp.Nonce).
				WithField("version_info", resp.SystemVersionInfo).
				WithField("currentVersion", currentVersion).
				Info("Sending incremental NACK discover request")

			fmt.Println(m.Format(nackReq))
			err := st.Send(nackReq)
			if err != nil {
				log.WithError(err).Fatal("failed to send NACK Discover Request")
			}

		} else {
			// We'll ACK our request.
			// The ResponseNonce field is what makes it an ACK.
			ackReq := &envoy_service_discovery_v3.DeltaDiscoveryRequest{
				TypeUrl:       typeURL,
				ResponseNonce: resp.Nonce,
				Node: &envoy_config_core_v3.Node{
					Id: nodeID,
				},
			}
			log.WithField("response_nonce", resp.Nonce).
				WithField("version_info", resp.SystemVersionInfo).
				WithField("currentVersion", currentVersion).
				Info("Sending incremental ACK discover request")
			fmt.Println(m.Format(ackReq))
			err := st.Send(ackReq)
			if err != nil {
				log.WithError(err).Fatal("failed to send ACK incremental Discover Request")
			}

		}
	}
}
