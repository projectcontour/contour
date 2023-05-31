## Failures to automatically set GOMAXPROCS are no longer fatal

In some (particularly local development) environments the [automaxprocs](https://github.com/uber-go/automaxprocs) library fails due to the cgroup namespace setup.
This failure is no longer fatal for Contour.
Contour will now simply log the error and continue with the automatic GOMAXPROCS detection ignored.
