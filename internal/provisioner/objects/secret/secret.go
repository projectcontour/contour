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

package secret

import (
	"context"
	"fmt"
	"strings"

	"github.com/projectcontour/contour/internal/certgen"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/pkg/certs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// generatedByVersionAnnotation is the key for the annotation that stores
// the version of Contour that the TLS secrets were generated by.
const generatedByVersionAnnotation = "projectcontour.io/generated-by-version"

// EnsureXDSSecrets ensures that mTLS secrets for Contour and Envoy exist
// and are up-to-date, i.e. have been generated with the current version.
func EnsureXDSSecrets(ctx context.Context, cli client.Client, contour *model.Contour, image string) error {
	desiredVersion := tagFromImage(image)

	if tlsSecretsExist(contour, cli, desiredVersion) {
		return nil
	}

	certs, err := certs.GenerateCerts(
		&certs.Configuration{
			Lifetime:  365,
			Namespace: contour.Namespace,
		},
	)
	if err != nil {
		return fmt.Errorf("error generating xDS TLS certificates: %w", err)
	}

	secrets := certgen.AsSecrets(contour.Namespace, contour.Name, certs)

	for _, secret := range secrets {
		// Add owner labels.
		if secret.Labels == nil {
			secret.Labels = model.OwnerLabels(contour)
		} else {
			for k, v := range model.OwnerLabels(contour) {
				secret.Labels[k] = v
			}
		}

		// Add annotation indicating the version the secret was
		// generated by, to ensure that we're rotating the secrets
		// on upgrade.
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[generatedByVersionAnnotation] = tagFromImage(image)

		if err := cli.Create(ctx, secret); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("error creating secret: %w", err)
			}

			if err := cli.Update(ctx, secret); err != nil {
				return fmt.Errorf("error updating secret: %w", err)
			}
		}
	}

	return nil
}

// tagFromImage returns the tag from the provided image or an
// empty string if the image does not contain a tag.
func tagFromImage(image string) string {
	if strings.Contains(image, ":") {
		parsed := strings.Split(image, ":")
		return parsed[1]
	}
	return ""
}

func tlsSecretsExist(contour *model.Contour, cli client.Client, generatedByVersion string) bool {
	for _, secretName := range []string{contour.Name + "-contourcert", contour.Name + "-envoycert"} {
		s := &corev1.Secret{}

		key := client.ObjectKey{
			Namespace: contour.Namespace,
			Name:      secretName,
		}

		if err := cli.Get(context.Background(), key, s); err != nil {
			return false
		}

		if s.Annotations[generatedByVersionAnnotation] != generatedByVersion {
			return false
		}
	}

	return true
}

func EnsureXDSSecretsDeleted(ctx context.Context, cli client.Client, contour *model.Contour) error {
	for _, secretName := range []string{contour.Name + "-contourcert", contour.Name + "-envoycert"} {
		s := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: contour.Namespace,
				Name:      secretName,
			},
		}

		if err := cli.Delete(context.Background(), s); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
