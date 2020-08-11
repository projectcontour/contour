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

// Package e2e provides end-to-end tests.
package e2e

import (
	"context"
	"math/rand"
	"net"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/fixture"
	cgrpc "github.com/projectcontour/contour/internal/grpc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	routeType    = resource.RouteType
	secretType   = resource.SecretType
	statsAddress = "0.0.0.0"
	statsPort    = 8002
)

func setup(t *testing.T, opts ...interface{}) (cache.ResourceEventHandler, *grpc.ClientConn, func()) {
	t.Parallel()

	log := fixture.NewTestLogger(t)

	et := &contour.EndpointsTranslator{
		FieldLogger: log,
	}

	conf := contour.ListenerConfig{}
	for _, opt := range opts {
		if opt, ok := opt.(func(*contour.ListenerConfig)); ok {
			opt(&conf)
		}
	}

	resources := []contour.ResourceCache{
		contour.NewListenerCache(conf, statsAddress, statsPort),
		&contour.SecretCache{},
		&contour.RouteCache{},
		&contour.ClusterCache{},
	}

	r := prometheus.NewRegistry()

	rand.Seed(time.Now().Unix())

	eh := &contour.EventHandler{
		Observer: dag.ComposeObservers(contour.ObserversOf(resources)...),
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				FieldLogger: log,
			},
		},
		StatusClient:    &k8s.StatusCacher{},
		FieldLogger:     log,
		Sequence:        make(chan int, 1),
		HoldoffDelay:    time.Duration(rand.Intn(100)) * time.Millisecond,
		HoldoffMaxDelay: time.Duration(rand.Intn(500)) * time.Millisecond,
	}

	for _, opt := range opts {
		if opt, ok := opt.(func(*contour.EventHandler)); ok {
			opt(eh)
		}
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	check(t, err)
	// Resource types in xDS v2.
	srv := cgrpc.NewAPI(log, append(contour.ResourcesOf(resources), et), r)

	var g workgroup.Group

	g.Add(func(stop <-chan struct{}) error {
		go func() {
			<-stop
			srv.GracefulStop()
		}()

		return srv.Serve(l) // srv now owns l and will close l before returning
	})
	g.Add(eh.Start())

	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	check(t, err)

	rh := &resourceEventHandler{
		EventHandler:        eh,
		EndpointsTranslator: et,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- g.Run(ctx)
	}()

	return rh, cc, func() {
		// close client connection
		cc.Close()

		// stop server
		cancel()

		<-done
	}
}

// resourceEventHandler composes a contour.Translator and a contour.EndpointsTranslator
// into a single ResourceEventHandler type.
type resourceEventHandler struct {
	*contour.EventHandler
	*contour.EndpointsTranslator
}

func (r *resourceEventHandler) OnAdd(obj interface{}) {
	switch obj.(type) {
	case *v1.Endpoints:
		r.EndpointsTranslator.OnAdd(obj)
	default:
		r.EventHandler.OnAdd(obj)
		<-r.EventHandler.Sequence
	}
}

func (r *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	switch newObj.(type) {
	case *v1.Endpoints:
		r.EndpointsTranslator.OnUpdate(oldObj, newObj)
	default:
		r.EventHandler.OnUpdate(oldObj, newObj)
		<-r.EventHandler.Sequence
	}
}

func (r *resourceEventHandler) OnDelete(obj interface{}) {
	switch obj.(type) {
	case *v1.Endpoints:
		r.EndpointsTranslator.OnDelete(obj)
	default:
		r.EventHandler.OnDelete(obj)
		<-r.EventHandler.Sequence
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func resources(t *testing.T, protos ...proto.Message) []*any.Any {
	t.Helper()
	var anys []*any.Any
	for _, a := range protos {
		anys = append(anys, protobuf.MustMarshalAny(a))
	}
	return anys
}

type grpcStream interface {
	Send(*v2.DiscoveryRequest) error
	Recv() (*v2.DiscoveryResponse, error)
}

func stream(t *testing.T, st grpcStream, req *v2.DiscoveryRequest) *v2.DiscoveryResponse {
	t.Helper()
	err := st.Send(req)
	check(t, err)
	resp, err := st.Recv()
	check(t, err)
	return resp
}

type Contour struct {
	*grpc.ClientConn
	*testing.T
}

func (c *Contour) Request(typeurl string, names ...string) *Response {
	c.Helper()
	var st grpcStream
	ctx := context.Background()
	switch typeurl {
	case secretType:
		sds := discovery.NewSecretDiscoveryServiceClient(c.ClientConn)
		sts, err := sds.StreamSecrets(ctx)
		c.check(err)
		st = sts
	default:
		c.Fatal("unknown typeURL: " + typeurl)
	}
	resp := c.sendRequest(st, &v2.DiscoveryRequest{
		TypeUrl:       typeurl,
		ResourceNames: names,
	})
	return &Response{
		Contour:           c,
		DiscoveryResponse: resp,
	}
}

func (c *Contour) sendRequest(stream grpcStream, req *v2.DiscoveryRequest) *v2.DiscoveryResponse {
	err := stream.Send(req)
	c.check(err)
	resp, err := stream.Recv()
	c.check(err)
	return resp
}

func (c *Contour) check(err error) {
	if err != nil {
		c.Fatal(err)
	}
}

type Response struct {
	*Contour
	*v2.DiscoveryResponse
}

func (r *Response) Equals(want *v2.DiscoveryResponse) {
	r.Helper()
	assert.Equal(r.T, want, r.DiscoveryResponse)
}
