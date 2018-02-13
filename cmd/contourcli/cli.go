// Copyright © 2017 Heptio
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
	"log"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"

	"google.golang.org/grpc"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
)

func main() {
	app := kingpin.New("contourcli", "A CLI client for the Heptio Contour Kubernetes ingress controller.")
	contour := app.Flag("contour", "contour host:port.").Default("127.0.0.1:8001").String()
	cds := app.Command("cds", "watch services.")
	eds := app.Command("eds", "watch endpoints.")
	lds := app.Command("lds", "watch listerners.")
	rds := app.Command("rds", "watch routes.")
	args := os.Args[1:]
	cmd := kingpin.MustParse(app.Parse(args))
	conn, err := grpc.Dial(*contour, grpc.WithInsecure())
	check(err)
	defer conn.Close()
	switch cmd {
	case cds.FullCommand():
		stream, err := v2.NewClusterDiscoveryServiceClient(conn).StreamClusters(context.Background())
		check(err)
		watchstream(stream)
	case eds.FullCommand():
		stream, err := v2.NewEndpointDiscoveryServiceClient(conn).StreamEndpoints(context.Background())
		check(err)
		watchstream(stream)
	case lds.FullCommand():
		stream, err := v2.NewListenerDiscoveryServiceClient(conn).StreamListeners(context.Background())
		check(err)
		watchstream(stream)
	case rds.FullCommand():
		stream, err := v2.NewRouteDiscoveryServiceClient(conn).StreamRoutes(context.Background())
		check(err)
		watchstream(stream)
	default:
		app.Usage(args)
		os.Exit(2)
	}
}

type stream interface {
	Recv() (*v2.DiscoveryResponse, error)
}

func watchstream(st stream) {
	m := proto.TextMarshaler{
		Compact:   false,
		ExpandAny: true,
	}
	for {
		resp, err := st.Recv()
		check(err)
		m.Marshal(os.Stdout, resp)
	}
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
