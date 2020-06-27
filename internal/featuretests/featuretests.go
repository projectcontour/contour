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

// Package featuretests provides end to end tests of specific features.
package featuretests

import (
	"context"
	"math/rand"
	"net"
	"sort"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/assert"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	cgrpc "github.com/projectcontour/contour/internal/grpc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	endpointType = resource.EndpointType // nolint:varcheck,deadcode
	clusterType  = resource.ClusterType
	routeType    = resource.RouteType
	listenerType = resource.ListenerType
	secretType   = resource.SecretType
	statsAddress = "0.0.0.0"
	statsPort    = 8002
)

type discardWriter struct{}

func (d *discardWriter) Write(buf []byte) (int, error) {
	return len(buf), nil
}

func setup(t *testing.T, opts ...func(*contour.EventHandler)) (cache.ResourceEventHandler, *Contour, func()) {
	return setupWithFallbackCert(t, "", "", opts...)
}

func setupWithFallbackCert(t *testing.T, fallbackCertName, fallbackCertNamespace string, opts ...func(*contour.EventHandler)) (cache.ResourceEventHandler, *Contour, func()) {
	t.Parallel()

	log := logrus.New()
	log.Out = new(discardWriter)

	et := &contour.EndpointsTranslator{
		FieldLogger: log,
	}

	r := prometheus.NewRegistry()
	ch := &contour.CacheHandler{
		Metrics:       metrics.NewMetrics(r),
		ListenerCache: contour.NewListenerCache(statsAddress, statsPort),
		FieldLogger:   log,
	}

	rand.Seed(time.Now().Unix())

	statusCache := &k8s.StatusCacher{}

	eh := &contour.EventHandler{
		IsLeader:        make(chan struct{}),
		CacheHandler:    ch,
		StatusClient:    statusCache,
		FieldLogger:     log,
		Sequence:        make(chan int, 1),
		HoldoffDelay:    time.Duration(rand.Intn(100)) * time.Millisecond,
		HoldoffMaxDelay: time.Duration(rand.Intn(500)) * time.Millisecond,
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				FieldLogger: log,
			},
			FallbackCertificate: &k8s.FullName{
				Name:      fallbackCertName,
				Namespace: fallbackCertNamespace,
			},
		},
	}

	for _, opt := range opts {
		opt(eh)
	}

	// Make this event handler win the leader election.
	close(eh.IsLeader)

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
	}, r)

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
		statusCache:         statusCache,
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

	return rh, &Contour{
			T:           t,
			ClientConn:  cc,
			statusCache: statusCache,
		}, func() {
			// close client connection
			cc.Close()

			// stop server
			close(stop)

			<-done
		}
}

// resourceEventHandler composes a contour.EventHandler and a contour.EndpointsTranslator
// into a single ResourceEventHandler type.
type resourceEventHandler struct {
	*contour.EventHandler
	*contour.EndpointsTranslator

	statusCache *k8s.StatusCacher
}

func (r *resourceEventHandler) OnAdd(obj interface{}) {
	if r.statusCache.IsCacheable(obj) {
		r.statusCache.Delete(obj)
	}

	switch obj.(type) {
	case *v1.Endpoints:
		r.EndpointsTranslator.OnAdd(obj)
	default:
		r.EventHandler.OnAdd(obj)
		<-r.EventHandler.Sequence
	}
}

func (r *resourceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	// Ensure that tests don't sample stale status.
	if r.statusCache.IsCacheable(oldObj) {
		r.statusCache.Delete(oldObj)
	}

	switch newObj.(type) {
	case *v1.Endpoints:
		r.EndpointsTranslator.OnUpdate(oldObj, newObj)
	default:
		r.EventHandler.OnUpdate(oldObj, newObj)
		<-r.EventHandler.Sequence
	}
}

func (r *resourceEventHandler) OnDelete(obj interface{}) {
	// Delete this object from the status cache before we make
	// the deletion visible.
	if r.statusCache.IsCacheable(obj) {
		r.statusCache.Delete(obj)
	}

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

// routeResources returns the given routes as a slice of any.Any
// resources, appropriately sorted.
func routeResources(t *testing.T, routes ...*v2.RouteConfiguration) []*any.Any {
	sort.Stable(sorter.For(routes))
	return resources(t, protobuf.AsMessages(routes)...)
}

func resources(t *testing.T, protos ...proto.Message) []*any.Any {
	t.Helper()
	anys := make([]*any.Any, 0, len(protos))
	for _, pb := range protos {
		anys = append(anys, protobuf.MustMarshalAny(pb))
	}
	return anys
}

type grpcStream interface {
	Send(*v2.DiscoveryRequest) error
	Recv() (*v2.DiscoveryResponse, error)
}

type statusResult struct {
	*Contour

	Err  error
	Have *projcontour.HTTPProxyStatus
}

// Equals asserts that the status result is not an error and matches
// the wanted status exactly.
func (s *statusResult) Equals(want projcontour.HTTPProxyStatus) *Contour {
	s.T.Helper()

	// We should never get an error fetching the status for an
	// object, so make it fatal if we do.
	if s.Err != nil {
		s.T.Fatalf(s.Err.Error())
	}

	assert.Equal(s.T, want, *s.Have)
	return s.Contour
}

// Like asserts that the status result is not an error and matches
// non-empty fields in the wanted status.
func (s *statusResult) Like(want projcontour.HTTPProxyStatus) *Contour {
	s.T.Helper()

	// We should never get an error fetching the status for an
	// object, so make it fatal if we do.
	if s.Err != nil {
		s.T.Fatalf(s.Err.Error())
	}

	if len(want.CurrentStatus) > 0 {
		assert.Equal(s.T,
			projcontour.HTTPProxyStatus{CurrentStatus: want.CurrentStatus},
			projcontour.HTTPProxyStatus{CurrentStatus: s.Have.CurrentStatus},
		)
	}

	if len(want.Description) > 0 {
		assert.Equal(s.T,
			projcontour.HTTPProxyStatus{Description: want.Description},
			projcontour.HTTPProxyStatus{Description: s.Have.Description},
		)
	}

	return s.Contour
}

type Contour struct {
	*grpc.ClientConn
	*testing.T

	statusCache *k8s.StatusCacher
}

// Status returns a statusResult object that can be used to assert
// on object status fields.
func (c *Contour) Status(obj interface{}) *statusResult {
	s, err := c.statusCache.GetStatus(obj)

	return &statusResult{
		Contour: c,
		Err:     err,
		Have:    s,
	}
}

// NoStatus asserts that the given object did not get any status set.
func (c *Contour) NoStatus(obj interface{}) *Contour {
	if _, err := c.statusCache.GetStatus(obj); err == nil {
		c.T.Errorf("found cached object status, wanted no status")
	}

	return c
}

func (c *Contour) Request(typeurl string, names ...string) *Response {
	c.Helper()
	var st grpcStream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	switch typeurl {
	case secretType:
		sds := discovery.NewSecretDiscoveryServiceClient(c.ClientConn)
		sts, err := sds.StreamSecrets(ctx)
		c.check(err)
		st = sts
	case routeType:
		rds := v2.NewRouteDiscoveryServiceClient(c.ClientConn)
		str, err := rds.StreamRoutes(ctx)
		c.check(err)
		st = str
	case clusterType:
		cds := v2.NewClusterDiscoveryServiceClient(c.ClientConn)
		stc, err := cds.StreamClusters(ctx)
		c.check(err)
		st = stc
	case listenerType:
		lds := v2.NewListenerDiscoveryServiceClient(c.ClientConn)
		stl, err := lds.StreamListeners(ctx)
		c.check(err)
		st = stl
	default:
		c.Fatal("unknown typeURL:", typeurl)
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

func (r *Response) Equals(want *v2.DiscoveryResponse) *Contour {
	r.Helper()
	assert.Equal(r.T, want, r.DiscoveryResponse)

	return r.Contour
}
