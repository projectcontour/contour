Contour now sets `GOMAXPROCS` to match the number of CPUs available to the container which results in lower and more stable CPU usage under high loads and where the container and node CPU counts differ significantly.
This is the default behavior but can be overridden by specifying `GOMAXPROCS` to a fixed value as an environment variable.
