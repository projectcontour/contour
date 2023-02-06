---
title: Using Gatekeeper as a validating admission controller with Contour
---

This tutorial demonstrates how to use [Gatekeeper](https://github.com/open-policy-agent/gatekeeper) as a validating admission controller for Contour.

Gatekeeper is a project that enables users to define flexible policies for Kubernetes resources using [Open Policy Agent (OPA)](https://www.openpolicyagent.org/) that are enforced when those resources are created/updated via the Kubernetes API.

The benefits of using Gatekeeper with Contour are:
- Immediate feedback for the user when they try to create an `HTTPProxy` with an invalid spec. Instead of having to check the `HTTPProxy`'s status after creation for a possible error message, the create is rejected and the user is immediately provided with a reason for the rejection.
- User-defined policies for `HTTPProxy` specs. For example, the Contour admin can define policies to enforce maximum limits on timeouts and retries, disallow certain FQDNs, etc.

## Prerequisites

- A Kubernetes cluster with a minimum version of 1.14 (to enable webhook timeouts for Gatekeeper).
- Cluster-admin permissions

## Deploy Contour

Run:

```bash
$ kubectl apply -f {{< param base_url >}}/quickstart/contour.yaml
```

This creates a `projectcontour` namespace and sets up Contour as a deployment and Envoy as a daemonset, with communication between them secured by mutual TLS.

Check the status of the Contour pods with this command:

```bash
$ kubectl -n projectcontour get pods -l app=contour
NAME                           READY   STATUS      RESTARTS   AGE
contour-8596d6dbd7-9nrg2       1/1     Running     0          32m
contour-8596d6dbd7-mmtc8       1/1     Running     0          32m
```

If installation was successful, all pods should reach `Running` status shortly.

## Deploy Gatekeeper

The following instructions are summarized from the [Gatekeeper documentation](https://github.com/open-policy-agent/gatekeeper#installation-instructions).
If you already have Gatekeeper running in your cluster, you can skip this section.

Run:

```bash
$ kubectl apply -f https://raw.githubusercontent.com/open-policy-agent/gatekeeper/master/deploy/gatekeeper.yaml
```

This creates a `gatekeeper-system` namespace and sets up the Gatekeeper controller manager and audit deployments using the latest Gatekeeper release.

Check the status of the Gatekeeper pods with this command:

```bash
$ kubectl -n gatekeeper-system get pods
NAME                                             READY   STATUS    RESTARTS   AGE
gatekeeper-audit-67dfc46db6-kjcmc                1/1     Running   0          40m
gatekeeper-controller-manager-7cbc758844-64hhn   1/1     Running   0          40m
gatekeeper-controller-manager-7cbc758844-c4dkd   1/1     Running   0          40m
gatekeeper-controller-manager-7cbc758844-xv9jn   1/1     Running   0          40m
```

If installation was successful, all pods should reach `Running` status shortly.

## Configure Gatekeeper

### Background

Gatekeeper uses the [OPA Constraint Framework](https://github.com/open-policy-agent/frameworks/tree/master/constraint) to define and enforce policies.
This framework has two key types: `ConstraintTemplate` and `Constraint`.
A `ConstraintTemplate` defines a reusable OPA policy, along with the parameters that can be passed to it when it is instantiated.
When a `ConstraintTemplate` is created, Gatekeeper automatically creates a custom resource definition (CRD) to represent it in the cluster.

A `Constraint` is an instantiation of a `ConstraintTemplate`, which tells Gatekeeper to apply it to specific Kubernetes resource types (e.g. `HTTPProxy`) and provides any relevant parameter values.
A `Constraint` is defined as an instance of the CRD representing the associated `ConstraintTemplate`.

We'll now look at some examples to make these concepts concrete.

### Configure resource caching

First, Gatekeeper needs to be configured to store all `HTTPProxy` resources in its internal cache, so that existing `HTTPProxy` resources can be referenced within constraint template policies.
This is essential for being able to define constraints that look across all `HTTPProxies` -- for example, to verify FQDN uniqueness.

Create a file called `config.yml` containing the following YAML:

```yaml
apiVersion: config.gatekeeper.sh/v1alpha1
kind: Config
metadata:
  name: config
  namespace: "gatekeeper-system"
spec:
  sync:
    syncOnly:
      - group: "projectcontour.io"
        version: "v1"
        kind: "HTTPProxy"
```

Apply it to the cluster:

```bash
$ kubectl apply -f config.yml
```

Note that if you already had Gatekeeper running in your cluster, you may already have the `Config` resource defined.
In that case, you'll need to edit the existing resource to add `HTTPProxy` to the `spec.sync.syncOnly` list.

### Configure HTTPProxy validations

The first constraint template and constraint that we'll define are what we'll refer to as a **validation**.
These are rules for `HTTPProxy` specs that Contour universally requires to be true.
In this example, we'll define a constraint template and constraint to enforce that all `HTTPProxies` must have a unique FQDN.

Create a file called `unique-fqdn-template.yml` containing the following YAML:

```yaml
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: httpproxyuniquefqdn
spec:
  crd:
    spec:
      names:
        kind: HTTPProxyUniqueFQDN
        listKind: HTTPProxyUniqueFQDNList
        plural: HTTPProxyUniqueFQDNs
        singular: HTTPProxyUniqueFQDN
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package httpproxy.uniquefqdn

        violation[{"msg": msg, "other": sprintf("%v/%v", [other.metadata.namespace, other.metadata.name])}] {
          got := input.review.object.spec.virtualhost.fqdn
          other := data.inventory.namespace[_]["projectcontour.io/v1"]["HTTPProxy"][_]
          other.spec.virtualhost.fqdn = got

          not same(other, input.review.object)
          msg := "HTTPProxy must have a unique spec.virtualhost.fqdn"
        }

        same(a, b) {
          a.metadata.namespace == b.metadata.namespace
          a.metadata.name == b.metadata.name
        }
```

Apply it to the cluster:

```bash
$ kubectl apply -f unique-fqdn-template.yml
```

Within a few seconds, you'll see that a corresponding CRD has been created in the cluster:

```bash
$ kubectl get crd httpproxyuniquefqdn.constraints.gatekeeper.sh
NAME                                            CREATED AT
httpproxyuniquefqdn.constraints.gatekeeper.sh   2020-08-13T16:08:57Z
```

Now, create a file called `unique-fqdn-constraint.yml` containing the following YAML:

```yaml
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: HTTPProxyUniqueFQDN
metadata:
  name: httpproxy-unique-fqdn
spec:
  match:
    kinds:
      - apiGroups: ["projectcontour.io"]
        kinds: ["HTTPProxy"]
```

Note that the `Kind` of this resource corresponds to the new CRD.

Apply it to the cluster:

```bash
$ kubectl apply -f unique-fqdn-constraint.yml
```

Now, let's create some `HTTPProxies` to see the validation in action.

Create a file called `httpproxies.yml` containing the following YAML:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: demo
  namespace: default
spec:
  virtualhost:
    fqdn: demo.projectcontour.io
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: demo2
  namespace: default
spec:
  virtualhost:
    fqdn: demo.projectcontour.io
```

Note that both `HTTPProxies` have the same FQDN.

Apply the YAML:

```bash
$ kubectl apply -f httpproxies.yml
```

You should see something like:
```
httpproxy.projectcontour.io/demo created
Error from server ([denied by httpproxy-unique-fqdn] HTTPProxy must have a unique FQDN): error when creating "httpproxies.yml": admission webhook "validation.gatekeeper.sh" denied the request: [denied by httpproxy-unique-fqdn] HTTPProxy must have a unique FQDN
```

The first `HTTPProxy` was created successfully, because there was not already an existing proxy with the `demo.projectcontour.io` FQDN.
However, when the second `HTTPProxy` was submitted, Gatekeeper rejected its creation because it used the same FQDN as the first one.

### Configure HTTPProxy policies

The next constraint template and constraint that we'll create are what we refer to as a **policy**.
These are rules for `HTTPProxy` specs that an individual Contour administrator may want to enforce for their cluster, but that are not explicitly required by Contour itself.
In this example, we'll define a constraint template and constraint to enforce that all `HTTPProxies` can be configured with at most five retries for any route.

Create a file called `retry-count-range-template.yml` containing the following YAML:

```yaml
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: httpproxyretrycountrange
spec:
  crd:
    spec:
      names:
        kind: HTTPProxyRetryCountRange
        listKind: HTTPProxyRetryCountRangeList
        plural: HTTPProxyRetryCountRanges
        singular: HTTPProxyRetryCountRange
      scope: Namespaced
      validation:
        openAPIV3Schema:
          properties:
            min:
              type: integer
            max: 
              type: integer
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package httpproxy.retrycountrange

        # build a set of all the retry count values
        retry_counts[val] {
          val := input.review.object.spec.routes[_].retryPolicy.count
        }

        # is there a retry count value that's greater than the allowed max?
        violation[{"msg": msg}] {
          retry_counts[_] > input.parameters.max
          msg := sprintf("retry count must be less than or equal to %v", [input.parameters.max])
        }

        # is there a retry count value that's less than the allowed min?
        violation[{"msg": msg}] {
          retry_counts[_] < input.parameters.min
          msg := sprintf("retry count must be greater than or equal to %v", [input.parameters.min])
        }
```

Apply it to the cluster:

```bash
$ kubectl apply -f retry-count-range-template.yml
```

Again, within a few seconds, you'll see that a corresponding CRD has been created in the cluster:

```bash
$ kubectl get crd httpproxyretrycountrange.constraints.gatekeeper.sh
NAME                                                 CREATED AT
httpproxyretrycountrange.constraints.gatekeeper.sh   2020-08-13T16:12:10Z
```

Now, create a file called `retry-count-range-constraint.yml` containing the following YAML:

```yaml
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: HTTPProxyRetryCountRange
metadata:
  name: httpproxy-retry-count-range
spec:
  match:
    kinds:
      - apiGroups: ["projectcontour.io"]
        kinds: ["HTTPProxy"]
    namespaces:
      - my-namespace
  parameters:
    max: 5
```

Note that for this `Constraint`, we've added a `spec.match.namespaces` field which defines that this policy should only be applied to `HTTPProxies` created in the `my-namespace` namespace.
If this `namespaces` matcher is not specified, then the `Constraint` applies to all namespaces.
You can read more about `Constraint` matchers on the [Gatekeeper website](https://github.com/open-policy-agent/gatekeeper#constraints).

Apply it to the cluster:

```bash
$ kubectl apply -f retry-count-range-constraint.yml
```

Now, let's create some `HTTPProxies` to see the policy in action.

Create a namespace called `my-namespace`:

```bash
$ kubectl create namespace my-namespace
namespace/my-namespace created
```

Create a file called `httpproxy-retries.yml` containing the following YAML:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: demo-retries
  namespace: my-namespace
spec:
  virtualhost:
    fqdn: retries.projectcontour.io
  routes:
    - conditions:
        - prefix: /foo
      services:
        - name: s1
          port: 80
      retryPolicy:
        count: 6
```

Apply the YAML:

```bash
$ kubectl apply -f httpproxy-retries.yml
```

You should see something like:
```
Error from server ([denied by httpproxy-retry-count-range] retry count must be less than or equal to 5): error when creating "proxy-retries.yml": admission webhook "validation.gatekeeper.sh" denied the request: [denied by httpproxy-retry-count-range] retry count must be less than or equal to 5
```

Now, change the `count` field on the last line of `httpproxy-retries.yml` to have a value of `5`. Save the file, and apply it again:

```bash
$ kubectl apply -f httpproxy-retries.yml
```

Now the `HTTPProxy` creates successfully*.

_* Note that the HTTPProxy is still marked invalid by Contour after creation because the service `s1` does not exist, but that's outside the scope of this guide._

Finally, create a file called `httpproxy-retries-default.yml` containing the following YAML:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: demo-retries
  namespace: default
spec:
  virtualhost:
    fqdn: default.retries.projectcontour.io
  routes:
    - conditions:
        - prefix: /foo
      services:
        - name: s1
          port: 80
      retryPolicy:
        count: 6
```

Remember that our `Constraint` was defined to apply only to the `my-namespace` namespace, so it should not block the creation of this proxy, even though it has a retry policy count outside the allowed range.

Apply the YAML:

```bash
$ kubectl apply -f httpproxy-retries-default.yml
```

The `HTTPProxy` creates successfully.

## Gatekeeper Audit

We've seen how Gatekeeper constraints can enforce constraints when a user tries to create a new `HTTPProxy`. Now let's look at how constraints can be applied to pre-existing resources in the cluster.

Gatekeeper has an audit functionality, that periodically (every `60s` by default) checks all existing resources against the relevant set of constraints. Any violations are reported in the `Constraint` custom resource's `status.violations` field. This allows an administrator to periodically review & correct any pre-existing misconfigurations, while not having to worry about breaking existing resources when rolling out a new or updated constraint.

To try this out, let's revisit the previous example, and change our constraint to allow a maximum retry count of four.

Edit `retry-count-range-constraint.yml` and change the `max` field to have a value of `4`. Save the file.

Apply it to the cluster:

```bash
$ kubectl apply -f retry-count-range-constraint.yml
```

We know that the `demo-retries` proxy has a route with a `retryPolicy.count` of `5`. This should now be invalid according to the updated constraint.

Wait up to `60s` for the next periodic audit to finish, then run:

```bash
$ kubectl describe httpproxyretrycountrange httpproxy-retry-count-range
```

You should see something like:

```
...
Status:
    ...
    Violations:
        Enforcement Action:  deny
        Kind:                HTTPProxy
        Message:             retry policy count must be less than or equal to 4
        Name:                demo-retries
        Namespace:           my-namespace
```

However, our `HTTPProxy` remains in the cluster and can continue to route requests, and the user can remediate the proxy to bring it inline with the policy on their own timeline.

## Next steps

Contour has a [growing library](https://github.com/projectcontour/contour/tree/main/examples/gatekeeper) of Gatekeeper constraint templates and constraints, for both **validations** and **policies**.

If you're using Gatekeeper, we recommend that you apply all of the **validations** we've defined, since these rules are already being checked internally by Contour and reported as status errors/invalid proxies.
Using the Gatekeeper constraints will only improve the user experience since users will get earlier feedback if their proxies are invalid.
The **validations** can be found in `examples/gatekeeper/validations`.


You should take more of a pick-and-choose approach to our sample **policies**, since every organization will have different policy needs.
Feel free to use any/all/none of them, and augment them with your own policies if applicable.
The sample **policies** can be found in `examples/gatekeeper/policies`.

And of course, if you do develop any new constraints that you think may be useful for the broader Contour community, we welcome contributions!
