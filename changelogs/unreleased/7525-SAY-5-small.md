Fix RetryPolicy numRetries=0 being silently coerced to 1 by populating num_retries explicitly when set, so HTTPProxy retry policies that explicitly request zero retries are honored.
