#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly OPERATION=$1
readonly CONTOUR_BENCH_CLUSTER_NAME=${CONTOUR_BENCH_CLUSTER_NAME:-contour-bench-$$}
readonly CONTOUR_BENCH_CLUSTER_ZONE=${CONTOUR_BENCH_CLUSTER_ZONE:-us-east1-d}
readonly CONTOUR_BENCH_CLUSTER_VERSION=${CONTOUR_BENCH_CLUSTER_VERSION:-latest}
readonly CONTOUR_BENCH_NODE_MACHINE_TYPE=${CONTOUR_BENCH_NODE_MACHINE_TYPE:-e2-standard-4}
readonly CONTOUR_BENCH_NUM_APP_POOL_NODES=${CONTOUR_BENCH_NUM_APP_POOL_NODES:-10}

# TODO: If credentials available in environment, log in.

function deploy() {
  num_nodes=$CONTOUR_BENCH_NUM_APP_POOL_NODES
  # Add one to the number of worker nodes for the node that will run contour.
  # This separate node from the application worker nodes will allow us to more
  # closely control contour resource limits.
  # We use this method rather than node pools since GKE repairs the cluster when
  # node pools are added, rendering it unusable for a few minutes.
  ((num_nodes+=1))

  gcloud container clusters create ${CONTOUR_BENCH_CLUSTER_NAME} \
    --cluster-version=${CONTOUR_BENCH_CLUSTER_VERSION} \
    --zone=${CONTOUR_BENCH_CLUSTER_ZONE} \
    --no-enable-autoupgrade \
    --no-enable-autorepair \
    --machine-type=${CONTOUR_BENCH_NODE_MACHINE_TYPE} \
    --num-nodes=$num_nodes \
    --addons=NodeLocalDNS

  # Enable system and workload monitoring to enable contour/envoy prometheus
  # endpoints.
  gcloud beta container clusters update ${CONTOUR_BENCH_CLUSTER_NAME} \
    --zone=${CONTOUR_BENCH_CLUSTER_ZONE} \
    --monitoring=SYSTEM,WORKLOAD

  gcloud container clusters get-credentials ${CONTOUR_BENCH_CLUSTER_NAME} --zone=${CONTOUR_BENCH_CLUSTER_ZONE}

  # In order to prevent GKE from immediately repairing our cluster, use only one node pool.
  nodes=$(kubectl get nodes | tail -n +2 | awk '{print $1}' | sort)
  # Label all nodes except one as app workload nodes.
  echo "${nodes}" | tail -n +2 | xargs -n1 -I{} kubectl label nodes {} projectcontour.bench-workload=app
  # Label remaining node as contour workload node.
  kubectl label nodes $(echo "${nodes}" | head -1) projectcontour.bench-workload=contour
}

function teardown() {
  gcloud container clusters delete ${CONTOUR_BENCH_CLUSTER_NAME} \
    --zone=${CONTOUR_BENCH_CLUSTER_ZONE} \
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
