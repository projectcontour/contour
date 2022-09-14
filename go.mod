module github.com/projectcontour/contour

go 1.16

require (
	github.com/ahmetb/gen-crd-api-reference-docs v0.3.0
	github.com/bombsimon/logrusr/v2 v2.0.1
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.8.0+incompatible
	github.com/envoyproxy/go-control-plane v0.10.3-0.20220715065308-8bcd7ee0191a
	github.com/go-logr/logr v1.2.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.7
	github.com/google/go-github/v39 v39.0.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/imdario/mergo v0.3.12
	github.com/jetstack/cert-manager v1.5.1
	github.com/onsi/ginkgo/v2 v2.0.0
	github.com/onsi/gomega v1.18.1
	github.com/projectcontour/yages v0.1.0
	github.com/prometheus/client_golang v1.12.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.32.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.1
	github.com/tsaarni/certyaml v0.9.0
	github.com/vektra/mockery/v2 v2.10.0
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	gonum.org/v1/plot v0.10.0
	google.golang.org/genproto v0.0.0-20220329172620-7be39ac1afc7
	google.golang.org/grpc v1.48.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.24.2
	k8s.io/apiextensions-apiserver v0.24.2
	k8s.io/apimachinery v0.24.2
	k8s.io/client-go v0.24.2
	k8s.io/klog/v2 v2.60.1
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/controller-runtime v0.12.1
	sigs.k8s.io/controller-tools v0.7.0
	sigs.k8s.io/gateway-api v0.5.1-0.20220822185609-d05e07b14156
	sigs.k8s.io/kustomize/kyaml v0.10.17
)
