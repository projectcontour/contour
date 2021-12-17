#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

CLUSTER_NAME=contour-bench
CLUSTER_ZONE=us-east1-d
CLUSTER_VERSION=latest
NODE_MACHINE_TYPE=e2-standard-4
NUM_APP_POOL_NODES=10

# TODO: If credentials available in environment, log in.

gcloud container clusters create ${CLUSTER_NAME} \
  --cluster-version=${CLUSTER_VERSION} \
  --zone=${CLUSTER_ZONE} \
  --no-enable-autoupgrade \
  --machine-type=${NODE_MACHINE_TYPE} \
  --num-nodes=${NUM_APP_POOL_NODES} \
  --addons=NodeLocalDNS \

# Enable system and workload monitoring to enable contour/envoy prometheus
# endpoints.
gcloud beta container clusters update ${CLUSTER_NAME} \
  --zone=${CLUSTER_ZONE} \
  --monitoring=SYSTEM,WORKLOAD

gcloud container clusters get-credentials ${CLUSTER_NAME} --zone=${CLUSTER_ZONE}

# Label default pool nodes
kubectl label nodes --all projectcontour.bench-workload=app

# Create Contour node-pool
gcloud container node-pools create contour-pool \
  --cluster=${CLUSTER_NAME} \
  --zone ${CLUSTER_ZONE} \
  --no-enable-autoupgrade \
  --machine-type=${NODE_MACHINE_TYPE} \
  --num-nodes=1 \
  --node-labels=projectcontour.bench-workload=contour
