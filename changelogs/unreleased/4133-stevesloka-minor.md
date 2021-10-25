### Default status on HTTPProxy resources

When a new HTTPProxy is created, if Contour isn't yet running or
functioning properly, then no status is set on the resource. 
Defaults of "NotReconciled/Waiting for controller" are now applied 
to any new object until an instance of Contour accepts the
object and updates the status.
