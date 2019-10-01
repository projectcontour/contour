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
	envoy "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/heptio/contour/apis/generated/clientset/versioned/fake"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/dag"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/heptio/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	endpointType = envoy.EndpointType
	clusterType  = envoy.ClusterType
	routeType    = envoy.RouteType
	listenerType = envoy.ListenerType
	secretType   = envoy.SecretType
	statsAddress = "0.0.0.0"
	statsPort    = 8002
)

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(buf []byte) (int, error) {
	t.Logf("%s", buf)
	return len(buf), nil
}

type discardWriter struct {
}

func (d *discardWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}

func setup(t *testing.T, opts ...func(*contour.EventHandler)) (cache.ResourceEventHandler, *grpc.ClientConn, func()) {
	t.Parallel()

	log := logrus.New()
	log.Out = &testWriter{t}

	et := &contour.EndpointsTranslator{
		FieldLogger: log,
	}

	r := prometheus.NewRegistry()
	ch := &contour.CacheHandler{
		IngressRouteStatus: &k8s.IngressRouteStatus{
			Client: fake.NewSimpleClientset(),
		},
		Metrics:       metrics.NewMetrics(r),
		ListenerCache: contour.NewListenerCache(statsAddress, statsPort),
		FieldLogger:   log,
	}

	rand.Seed(time.Now().Unix())

	eh := &contour.EventHandler{
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				FieldLogger: log,
			},
		},
		CacheHandler:    ch,
		Metrics:         ch.Metrics,
		FieldLogger:     log,
		Sequence:        make(chan int, 1),
		HoldoffDelay:    time.Duration(rand.Intn(100)) * time.Millisecond,
		HoldoffMaxDelay: time.Duration(rand.Intn(500)) * time.Millisecond,
	}

	for _, opt := range opts {
		opt(eh)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	check(t, err)
	discard := logrus.New()
	discard.Out = new(discardWriter)
	// Resource types in xDS v2.
	srv := cgrpc.NewAPI(discard, map[string]cgrpc.Resource{
		ch.ClusterCache.TypeURL():  &ch.ClusterCache,
		ch.RouteCache.TypeURL():    &ch.RouteCache,
		ch.ListenerCache.TypeURL(): &ch.ListenerCache,
		ch.SecretCache.TypeURL():   &ch.SecretCache,
		et.TypeURL():               et,
	})

	var g workgroup.Group

	g.Add(func(stop <-chan struct{}) error {
		done := make(chan error)
		go func() {
			done <- srv.Serve(l) // srv now owns l and will close l before returning
		}()
		<-stop
		srv.Stop()
		return <-done
	})
	g.Add(eh.Start())

	cc, err := grpc.Dial(l.Addr().String(), grpc.WithInsecure())
	check(t, err)

	rh := &resourceEventHandler{
		EventHandler:        eh,
		EndpointsTranslator: et,
	}

	stop := make(chan struct{})
	g.Add(func(_ <-chan struct{}) error {
		<-stop
		return nil
	})

	done := make(chan error)
	go func() {
		done <- g.Run()
	}()

	return rh, cc, func() {
		// close client connection
		cc.Close()

		// stop server
		close(stop)

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

func resources(t *testing.T, protos ...proto.Message) []types.Any {
	t.Helper()
	if len(protos) == 0 {
		return nil
	}
	anys := make([]types.Any, len(protos))
	for i, a := range protos {
		anys[i] = any(t, a)
	}
	return anys
}

func any(t *testing.T, pb proto.Message) types.Any {
	t.Helper()
	any, err := types.MarshalAny(pb)
	check(t, err)
	return *any
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
	assertEqual(r.T, want, r.DiscoveryResponse)
}

func assertEqual(t *testing.T, want, got *v2.DiscoveryResponse) {
	t.Helper()
	m := proto.TextMarshaler{Compact: true, ExpandAny: true}
	a := m.Text(want)
	b := m.Text(got)
	if a != b {
		m := proto.TextMarshaler{
			Compact:   false,
			ExpandAny: true,
		}
		t.Fatalf("\nexpected:\n%v\ngot:\n%v", m.Text(want), m.Text(got))
	}
}

func u32(val int) *types.UInt32Value { return &types.UInt32Value{Value: uint32(val)} }
