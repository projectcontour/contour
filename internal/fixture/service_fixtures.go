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

package fixture

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var ServiceRootsKuard = &v1.Service{
	ObjectMeta: ObjectMeta("roots/kuard"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceRootsHome = &v1.Service{
	ObjectMeta: ObjectMeta("roots/home"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo1 = &v1.Service{
	ObjectMeta: ObjectMeta("roots/foo1"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo2 = &v1.Service{
	ObjectMeta: ObjectMeta("roots/foo2"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceRootsFoo3InvalidPort = &v1.Service{
	ObjectMeta: ObjectMeta("roots/foo3"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     12345678,
		}},
	},
}

var ServiceMarketingGreen = &v1.Service{
	ObjectMeta: ObjectMeta("marketing/green"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceRootsNginx = &v1.Service{
	ObjectMeta: ObjectMeta("roots/nginx"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceTeamAKuard = &v1.Service{
	ObjectMeta: ObjectMeta("teama/kuard"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceTeamBKuard = &v1.Service{
	ObjectMeta: ObjectMeta("teamb/kuard"),
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}
