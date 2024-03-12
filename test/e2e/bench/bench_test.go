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

//go:build e2e

package bench

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/stretchr/testify/require"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/test/e2e"
)

var (
	f            = e2e.NewFramework(true)
	reportDir    string
	lbExternalIP string
	numServices  = 1000
)

func TestBench(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Benchmark tests")
}

var _ = BeforeSuite(func() {
	var found bool
	reportDir, found = os.LookupEnv("CONTOUR_BENCH_REPORT_DIR")
	require.True(f.T(), found, "Must provide CONTOUR_BENCH_REPORT_DIR env var")

	numServicesStr, found := os.LookupEnv("CONTOUR_BENCH_NUM_SERVICES")
	if found {
		var err error
		numServices, err = strconv.Atoi(numServicesStr)
		require.NoError(f.T(), err, "failed to parse CONTOUR_BENCH_NUM_SERVICES as integer")
	}

	// Add node selectors to Contour and Envoy resources.
	f.Deployment.ContourDeployment.Spec.Template.Spec.NodeSelector = map[string]string{
		"projectcontour.bench-workload": "contour",
	}
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.NodeSelector = map[string]string{
		"projectcontour.bench-workload": "app",
	}
	// Add resource limits to Contour Deployment.
	f.Deployment.ContourDeployment.Spec.Template.Spec.Containers[0].Resources = core_v1.ResourceRequirements{
		Limits: core_v1.ResourceList{
			core_v1.ResourceCPU:    resource.MustParse("1"),
			core_v1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	// Add metrics port to Envoy DaemonSet.
	f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.Containers[1].Ports = append(
		f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.Containers[1].Ports,
		core_v1.ContainerPort{
			Name:          "metrics",
			HostPort:      8002,
			ContainerPort: 8002,
			Protocol:      core_v1.ProtocolTCP,
		},
	)

	require.NoError(f.T(), f.Deployment.EnsureResourcesForInclusterContour(true))

	require.Eventually(f.T(), func() bool {
		s := &core_v1.Service{}
		if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(f.Deployment.EnvoyService), s); err != nil {
			return false
		}
		if len(s.Status.LoadBalancer.Ingress) == 0 {
			return false
		}
		lbExternalIP = s.Status.LoadBalancer.Ingress[0].IP
		return true
	}, f.RetryTimeout, f.RetryInterval)
})

var _ = AfterSuite(func() {
	require.NoError(f.T(), f.Deployment.DeleteResourcesForInclusterContour())
})

var _ = Describe("Benchmark", func() {
	f.NamespacedTest("sequential-service-creation", func(namespace string) {
		Context("with many services created sequentially", func() {
			deployApp := func(name string) {
				deployment := &apps_v1.Deployment{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      name,
					},
					Spec: apps_v1.DeploymentSpec{
						Selector: &meta_v1.LabelSelector{
							MatchLabels: map[string]string{"app.kubernetes.io/name": name},
						},
						Template: core_v1.PodTemplateSpec{
							ObjectMeta: meta_v1.ObjectMeta{
								Labels: map[string]string{"app.kubernetes.io/name": name},
							},
							Spec: core_v1.PodSpec{
								NodeSelector: map[string]string{
									"projectcontour.bench-workload": "app",
								},
								Containers: []core_v1.Container{
									{
										Name:  "conformance-echo",
										Image: "gcr.io/k8s-staging-ingressconformance/echoserver@sha256:dc59c3e517399b436fa9db58f16506bd37f3cd831a7298eaf01bd5762ec514e1",
										Env: []core_v1.EnvVar{
											{
												Name:  "INGRESS_NAME",
												Value: name,
											},
											{
												Name:  "SERVICE_NAME",
												Value: name,
											},
											{
												Name: "POD_NAME",
												ValueFrom: &core_v1.EnvVarSource{
													FieldRef: &core_v1.ObjectFieldSelector{
														FieldPath: "metadata.name",
													},
												},
											},
											{
												Name: "NAMESPACE",
												ValueFrom: &core_v1.EnvVarSource{
													FieldRef: &core_v1.ObjectFieldSelector{
														FieldPath: "metadata.namespace",
													},
												},
											},
										},
										Ports: []core_v1.ContainerPort{
											{
												Name:          "http-api",
												ContainerPort: 3000,
											},
										},
										ReadinessProbe: &core_v1.Probe{
											ProbeHandler: core_v1.ProbeHandler{
												HTTPGet: &core_v1.HTTPGetAction{
													Path: "/health",
													Port: intstr.FromInt(3000),
												},
											},
										},
									},
								},
							},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), deployment))

				service := &core_v1.Service{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      name,
					},
					Spec: core_v1.ServiceSpec{
						Ports: []core_v1.ServicePort{
							{
								Name:       "http",
								Port:       80,
								TargetPort: intstr.FromString("http-api"),
							},
						},
						Selector: map[string]string{"app.kubernetes.io/name": name},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), service))

				// Wait for deployment availability before we continue.
				require.Eventually(f.T(), func() bool {
					d := &apps_v1.Deployment{}
					if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(deployment), d); err != nil {
						return false
					}
					for _, c := range d.Status.Conditions {
						return c.Type == apps_v1.DeploymentAvailable && c.Status == core_v1.ConditionTrue
					}
					return false
				}, time.Minute*2, f.RetryInterval)
			}

			deployHTTPProxy := func(name string) {
				p := &contour_v1.HTTPProxy{
					ObjectMeta: meta_v1.ObjectMeta{
						Namespace: namespace,
						Name:      name,
					},
					Spec: contour_v1.HTTPProxySpec{
						VirtualHost: &contour_v1.VirtualHost{
							Fqdn: name + ".projectcontour.io",
						},
						Routes: []contour_v1.Route{
							{
								Services: []contour_v1.Service{
									{
										Name: name,
										Port: 80,
									},
								},
							},
						},
					},
				}
				require.NoError(f.T(), f.Client.Create(context.TODO(), p))
			}

			var experiment *gmeasure.Experiment

			BeforeEach(func() {
				experiment = gmeasure.NewExperiment("sequential-service-creation")
				AddReportEntry(experiment.Name, experiment)

				// Warm up Envoy on each worker node to ensure no outliers.
				deployApp("warm-up")
				deployHTTPProxy("warm-up")
				nodes := &core_v1.NodeList{}
				labelSelector := &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(f.Deployment.EnvoyDaemonSet.Spec.Template.Spec.NodeSelector),
				}
				require.NoError(f.T(), f.Client.List(context.Background(), nodes, labelSelector))

				for _, node := range nodes.Items {
					nodeExternalIP := ""
					for _, a := range node.Status.Addresses {
						if a.Type == core_v1.NodeExternalIP {
							nodeExternalIP = a.Address
						}
					}
					require.NotEmpty(f.T(), nodeExternalIP, "did not find an external ip for node %s", node.Name)

					res, ok := f.HTTP.RequestUntil(&e2e.HTTPRequestOpts{
						Host:        "warm-up.projectcontour.io",
						OverrideURL: "http://" + nodeExternalIP,
						Condition:   e2e.HasStatusCode(200),
					})
					require.NotNil(f.T(), res, "request never succeeded")
					require.Truef(f.T(), ok, "expected 200 response code, got %d", res.StatusCode)
				}
			})

			AfterEach(func() {
				durations := experiment.Get("time_to_ready").Durations
				writeCSV(durations)
				drawScatterPlot(durations)
			})

			Specify("time to service available does not increase drastically", func() {
				const (
					maxRetries = 500
				)

				client := &http.Client{
					Timeout: time.Millisecond * 500,
				}
				for i := range numServices {
					appName := fmt.Sprintf("echo-%d", i)
					deployApp(appName)
					req, err := http.NewRequest(http.MethodGet, "http://"+lbExternalIP, nil)
					require.NoError(f.T(), err, "error creating HTTP request")
					req.Host = appName + ".projectcontour.io"

					deployHTTPProxy(appName)

					// Nothing else should happen here before measuring time to ready.

					experiment.MeasureDuration("time_to_ready", func() {
						retries := 0
						available := false
						for !available {
							require.Less(f.T(), retries, maxRetries, "reached maximum retries for host:", req.Host)
							res, err := client.Do(req)
							if err == nil && res.StatusCode == http.StatusOK {
								available = true
							}
							retries++
							time.Sleep(time.Millisecond * 100)
						}
					}, gmeasure.Annotation(appName))
				}
			})
		})
	})
})

func writeCSV(durations []time.Duration) {
	csv, err := os.Create(filepath.Join(reportDir, "sequential-service-creation.csv"))
	require.NoError(f.T(), err)
	defer func() {
		require.NoError(f.T(), csv.Close())
	}()

	// Write CSV header
	fmt.Fprintln(csv, "num_services,time_to_ready")

	for i, d := range durations {
		// Write each line of data.
		fmt.Fprintf(csv, "%d,%d\n", i+1, d)
	}
}

func drawScatterPlot(durations []time.Duration) {
	p := plot.New()
	p.Title.Text = "Sequential Service Creation"
	p.X.Label.Text = "num_services"
	p.Y.Label.Text = "time_to_ready"
	p.Add(plotter.NewGrid())

	points := make(plotter.XYs, len(durations))
	for i, d := range durations {
		points[i].X = float64(i + 1)
		points[i].Y = d.Seconds()
	}

	s, err := plotter.NewScatter(points)
	require.NoError(f.T(), err)
	s.GlyphStyle.Color = color.RGBA{R: 9, G: 87, B: 245, A: 1}

	p.Add(s)

	require.NoError(f.T(), p.Save(8*vg.Inch, 6*vg.Inch, filepath.Join(reportDir, "sequential-service-creation.png")))
}
