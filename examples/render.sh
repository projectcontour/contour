#! /usr/bin/env bash

ls contour/*.yaml | \
  xargs cat render/gen-warning.yaml | \
  sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' \
  > render/contour.yaml
