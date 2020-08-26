# Gatekeeper examples

This directory contains example YAML to configure [Gatekeeper](https://github.com/open-policy-agent/gatekeeper) to work with Contour.
It has the following subdirectories:
- **policies/** has sample `ConstraintTemplates` and `Constraints` implementing rules that a Contour user *may* want to enforce for their clusters, but that are not required for Contour to function. You should take a pick-and-choose approach to the contents of this directory, and should modify/extend them to meet your unique needs.
- **validations/** has `ConstraintTemplates` and `Constraints` implementing rules that Contour universally requires to be true. If you're using Contour and Gatekeeper, we recommend you use all of the rules defined in this directory.

See the [Gatekeeper guide](https://projectcontour.io/guides/gatekeeper/) for more information.
