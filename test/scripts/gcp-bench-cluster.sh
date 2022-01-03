#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly OPERATION=$1
readonly CLUSTER_NAME=${CLUSTER_NAME:-contour-bench-$$}
readonly CLUSTER_ZONE=${CLUSTER_ZONE:-us-east1-d}
readonly CLUSTER_VERSION=${CLUSTER_VERSION:-latest}
readonly NODE_MACHINE_TYPE=${NODE_MACHINE_TYPE:-e2-standard-4}
readonly NUM_APP_POOL_NODES=${NUM_APP_POOL_NODES:-10}

# TODO: If credentials available in environment, log in.

function deploy() {
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
}

function teardown() {
  gcloud container clusters delete ${CLUSTER_NAME} \
    --zone=${CLUSTER_ZONE} \
    --quiet
}

case ${OPERATION} in
  deploy)
    echo "Deploying benchmark cluster"
    deploy
    ;;
  teardown)
    echo "Tearing down benchmark cluster"
    teardown
    ;;
  *)
    echo "Error: Invalid operation ${OPERATION}, must be one of deploy or teardown"
    exit 1
    ;;
esac
