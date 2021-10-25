### Allow retry policy, num retries to be zero 

The field, NumRetries (e.g. count), in the RetryPolicy allows for a zero to be
specified, however Contour's internal logic would see that as "undefined"
and set it back to the Envoy default of 1. This would never allow the value of 
zero to be set. Users can set the value to be -1 which will represent disabling 
the retry count. If not specified or set to zero, then the Envoy default value 
of 1 is used. 