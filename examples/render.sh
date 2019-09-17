#! /bin/sh

ls contour/*.yaml | xargs cat render/gen-warning.yaml > render/contour.yaml
