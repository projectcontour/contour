## http buffer filter configuration

Introduce two optional config parameter `http-buffer-max-request-bytes`, with default values set to `0`. If the value is non-zero, an HTTP buffer filter will be added to the HTTP filter chain immediately after the `DefaultFilters()` with the `max_request_bytes` parameter. This configuration allows setting the buffer filter for the entire HTTP listeners only (both http and https). 
