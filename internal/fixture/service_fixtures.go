package fixture

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var ServiceKuard = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "kuard",
		Namespace: SecretRootsNS.Namespace,
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceHome = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "home",
		Namespace: ServiceKuard.Namespace,
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceFoo2 = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo2",
		Namespace: ServiceKuard.Namespace,
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     8080,
		}},
	},
}

var ServiceFoo3InvalidPort = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo3",
		Namespace: ServiceKuard.Namespace,
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     12345678,
		}},
	},
}

var ServiceGreenMarketing = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "green",
		Namespace: "marketing",
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceNginx = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: ServiceKuard.Namespace,
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Protocol: "TCP",
			Port:     80,
		}},
	},
}

var ServiceKuardTeamA = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "kuard",
		Namespace: "teama",
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}

var ServiceKuardTeamB = &v1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "kuard",
		Namespace: "teamb",
	},
	Spec: v1.ServiceSpec{
		Ports: []v1.ServicePort{{
			Name:       "http",
			Protocol:   "TCP",
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		}},
	},
}
