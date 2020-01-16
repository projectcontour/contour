# Move Contour to use kubebuilder instead of informers

Status: Draft

Contour's method of interaction with Kubernetes is two years old, and the community has moved on from Informers.
This proposal suggests that we should move to using Kubebuilder [controller-runtime][1] for interacting with Kubernetes.
This will make extending Contour with additional CRDs as we need to more straightforward.

## Goals

- Migrate Contour to use Kubebuilder's [controller-runtime][1] library instead of informers.

## Non Goals

- Implement the new service-api types as part of this proposal
- Use Kubebuilder for code autogeneration. We've already done equivalent work.

## Background

Two years ago, @davecheney built Contour to use client-go Informer code to talk to Kubernetes.
At the time, this was a sensible decision, as there were no well-developed frameworks or common methods for interacting with Kubernetes.
Since then, the community has done a large amount of work to simplify the construction of controllers, currently embodied in the kubebuilder [controller-runtime][1] library.

This has become an issue now because we are looking at adding support for further CRDs, (the SIG-Network service-apis) and they are not shipping with Informers generated.
Moving to the same framework as the new service-api types will allow us to easily consume them, as well as add any other new types without requiring generated Informers.

In addition, moving to using controller-runtime means that we have access to frameworks for helping with:

- leader election
- defaulting values for CRDs
- webhooks to translate between CRD versions (related to #2068)

## High-Level Design

In order to avoid a long-running feature branch, I propose a multi-phase approach that can be broadly summarized as:

- Move things that interact with the Kubernetes apiserver to use more standard implementations (particularly Caches, but there may be others). This will involve a few changes at least.
- Move `contour serve` to using the controller-runtime framework for its serving.

In particular, the controller-runtime framework requires Controllers to have a Reconcile function that is supplied the details of an object that has changed, and the controller is expected to perform whatever actions it needs to to ensure that the new object correctly reflects the state of the world.
It's a reconciliation loop in a function, in other words.

Currently Contour uses an Eventhandler with Add, Update and Delete methods for each type it supports for this function, also with an added Cache that only keeps things that the Contour DAG should care about.
A major part of this work will be converting the old Informer code that fires separate events on add, update, and delete, to using a Reconcile-style function that idempotently looks at the current state and figures out what to do.

### Moving towards more standard Kubernetes interactions

Firstly, we'll review the various caches that Contour uses to see if any should be removed or collapsed back into the standard client-go Cache type.

Then, We'll need to move the code that performs changes from Add, Update and Delete, into a singular Reconcile function that figures out what has changed and does that.
This will require some changes to 

The detailed design section is omitted because this work will of necessity need to change as it goes.

## Alternatives Considered

We could generate our own code for Informers for the service-api types and any other types we need to add.
However, this will miss the opportunity to keep our code up to date with the rest of the community, and add extra overhead to the process of consuming other API types.

We could also use the dynamic client, which seems to be what controller-runtime uses, but that will still need a large amount of refactoring, for less end benefit.

[1]: https://github.com/kubernetes-sigs/controller-runtime
