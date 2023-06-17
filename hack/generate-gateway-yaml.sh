#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Wrap sed to deal with GNU and BSD sed flags.
run::sed() {
    local -r vers="$(sed --version < /dev/null 2>&1 | grep -q GNU && echo gnu || echo bsd)"
    case "$vers" in
        gnu) sed -i "$@" ;;
        *) sed -i '' "$@" ;;
    esac
}


kubectl kustomize -o examples/gateway/00-crds.yaml "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=${GATEWAY_API_VERSION}"
echo "Generating Gateway API webhook documents..."
curl -s -o examples/gateway/00-namespace.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/0-namespace.yaml
curl -s -o examples/gateway/01-admission_webhook.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/admission_webhook.yaml
curl -s -o examples/gateway/02-certificate_config.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/certificate_config.yaml

patch -p1 <<EOF
diff --git a/examples/gateway/01-admission_webhook.yaml b/examples/gateway/01-admission_webhook.yaml
index adf2cfc0..63ad420a 100644
--- a/examples/gateway/01-admission_webhook.yaml
+++ b/examples/gateway/01-admission_webhook.yaml
@@ -44,7 +44,7 @@ metadata:
   labels:
     name: gateway-api-admission-server
 spec:
-  replicas: 1
+  replicas: 2
   selector:
     matchLabels:
       name: gateway-api-admission-server
@@ -80,6 +80,12 @@ spec:
           readOnly: true
         securityContext:
           readOnlyRootFilesystem: true
+        livenessProbe:
+          tcpSocket:
+            port: 8443
+        readinessProbe:
+          tcpSocket:
+            port: 8443
       volumes:
       - name: webhook-certs
         secret:
EOF
