#! /bin/sh

ls ds-json-v1/*.yaml | xargs cat common/gen-warning.yaml > render/daemonset-rbac.yaml
ls ds-json-v1/*.yaml | grep -v rbac | xargs cat common/gen-warning.yaml > render/daemonset-norbac.yaml

ls deployment-json-v1/*.yaml | xargs cat common/gen-warning.yaml > render/deployment-rbac.yaml
ls deployment-json-v1/*.yaml | grep -v rbac | xargs cat common/gen-warning.yaml > render/deployment-norbac.yaml
