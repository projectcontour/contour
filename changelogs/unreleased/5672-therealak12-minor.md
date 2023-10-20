## Contour now waits for the cache sync before starting the DAG rebuild and XDS server

Before this, we only waited for informer caches to sync but didn't wait for delivering the events to subscribed handlers.
Now contour waits for the initial list of Kubernetes objects to be cached and processed by handlers (using the returned `HasSynced` methods)
and then starts building its DAG and serving XDS. 
