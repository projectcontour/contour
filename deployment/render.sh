#! /bin/sh

ls ds-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/daemonset-rbac.yaml
ls ds-grpc-v2/*.yaml | grep -v rbac | xargs cat common/gen-warning.yaml > render/daemonset-norbac.yaml

ls deployment-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/deployment-rbac.yaml
ls deployment-grpc-v2/*.yaml | grep -v rbac | xargs cat common/gen-warning.yaml > render/deployment-norbac.yaml
