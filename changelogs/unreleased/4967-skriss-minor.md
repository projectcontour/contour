## shutdown-manager sidecar container liveness probe removed

The liveness probe has been removed from the Envoy pods' shutdown-manager sidecar container.
This change is to mitigate a problem where when the liveness probe fails, the shutdown-manager container is restarted by itself.
This ultimately has the unintended effect of causing the envoy container to be stuck indefinitely in a "DRAINING" state and not serving traffic.
    
Overall, not having the liveness probe on the shutdown-manager container is less bad because envoy pods are less likely to get stuck in "DRAINING" indefinitely. 
In the worst case, during termination of an Envoy pod (due to upgrade, scaling, etc.), shutdown-manager is truly unresponsive, in which case the envoy container will simply terminate without first draining active connections.
If appropriate (i.e. during an upgrade), a new Envoy pod will then be created and re-added to the set of ready Envoys to load balance traffic to.
