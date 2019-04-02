#! /bin/sh

# Run correctly from any CWD
cd $(dirname $0)

ls ds-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/daemonset-rbac.yaml

ls deployment-grpc-v2/*.yaml | xargs cat common/gen-warning.yaml > render/deployment-rbac.yaml
