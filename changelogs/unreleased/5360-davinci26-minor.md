## Adds critical level for access logging

Similar motivation with https://github.com/projectcontour/contour/pull/4331/files
to reduce the volume of logs for big installations. Critical level produces access
logs for response status >= 500
