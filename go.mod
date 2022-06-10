module github.com/projectcontour/contour

go 1.15

require (
	github.com/ahmetb/gen-crd-api-reference-docs v0.3.0
	github.com/bombsimon/logrusr/v2 v2.0.1
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.8.0+incompatible
	github.com/envoyproxy/go-control-plane v0.10.2-0.20220112105034-1553555e45ad
	github.com/go-logr/logr v1.2.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v39 v39.0.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/imdario/mergo v0.3.12
	github.com/jetstack/cert-manager v1.5.1
	github.com/onsi/ginkgo/v2 v2.0.0
	github.com/onsi/gomega v1.17.0
	github.com/prometheus/client_golang v1.11.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.28.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tsaarni/certyaml v0.6.2
	github.com/vektra/mockery/v2 v2.10.0
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	gonum.org/v1/plot v0.10.0
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa
	google.golang.org/grpc v1.43.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.23.1
	k8s.io/apiextensions-apiserver v0.23.0
	k8s.io/apimachinery v0.23.1
	k8s.io/client-go v0.23.1
	k8s.io/klog/v2 v2.30.0
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/controller-runtime v0.11.0
	sigs.k8s.io/controller-tools v0.6.2
	sigs.k8s.io/gateway-api v0.4.3
	sigs.k8s.io/kustomize/kyaml v0.10.17
)
