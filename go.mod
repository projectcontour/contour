module github.com/projectcontour/contour

go 1.15

require (
	github.com/ahmetb/gen-crd-api-reference-docs v0.3.0
	github.com/bombsimon/logrusr v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/envoyproxy/go-control-plane v0.9.10-0.20211006050637-f76d23b38f14
	github.com/go-logr/logr v0.4.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v39 v39.0.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/jetstack/cert-manager v1.5.1
	github.com/onsi/ginkgo v1.16.5-0.20211011165036-638dfbc0fced
	github.com/onsi/gomega v1.16.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.26.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/tsaarni/certyaml v0.6.2
	github.com/vektra/mockery/v2 v2.9.4
	golang.org/x/net v0.0.0-20211209124913-491a49abca63 // indirect
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	gonum.org/v1/plot v0.10.0
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.0
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/klog/v2 v2.10.0
	k8s.io/utils v0.0.0-20210820185131-d34e5cb4466e
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/controller-tools v0.6.2
	sigs.k8s.io/gateway-api v0.4.0
	sigs.k8s.io/kustomize/kyaml v0.10.17
)
