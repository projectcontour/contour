#! /bin/sh

ls ds-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/daemonset-rbac.yaml

ls deployment-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/deployment-rbac.yaml
