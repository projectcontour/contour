package fixture

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var SecretRootsNS = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "ssl-cert",
		Namespace: "roots",
	},
	Type: v1.SecretTypeTLS,
	Data: map[string][]byte{
		v1.TLSCertKey:       []byte(CERTIFICATE),
		v1.TLSPrivateKeyKey: []byte(RSA_PRIVATE_KEY),
	},
}

var SecretContourNS = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "default-ssl-cert",
		Namespace: "projectcontour",
	},
	Type: v1.SecretTypeTLS,
	Data: SecretRootsNS.Data,
}

var FallbackSecret = &v1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "fallbacksecret",
		Namespace: "roots",
	},
	Type: v1.SecretTypeTLS,
	Data: map[string][]byte{
		v1.TLSCertKey:       []byte(CERTIFICATE),
		v1.TLSPrivateKeyKey: []byte(RSA_PRIVATE_KEY),
	},
}
