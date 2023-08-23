## Contour now waits for the cache sync before starting the DAG rebuild and XDS server

Contour will wait for the initial list of Kubernetes objects to be processed and cached and then starts building its DAG and
serving XDS.
