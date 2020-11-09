# Delegation

**Status**: _Informational_

Delegation is Contour's mechanism of controlling access to the route space of a set of virtual hosts.

Conceptually the idea of delegating control of a part of a thing to another person is straight forward.
To illustrate this we often use the analogy of DNS, a domain owner delegates control of a subdomain to another administrator.
This model seems to resonate with people as it is a close analog between something they understand--DNS--and something they want--limiting what changes someone else can make to objects in their Kubernetes cluster.

There is another way of thinking about delegation; file inclusion.
This model is closer to how delegation is implemented inside Contour.
Ignoring the face that the API server is not a file system, the `delegate:` key functions similar to `#include` in a programming language--at this point in processing the IngressRoute, insert the contents of the delegate.

These two models, DNS delegation, and file inclusion overlap in ways that are sometimes unexpected.
This document explores their differences.

## DNS as a model for delegation

When describing how Contour's delegation model allows ingress owners to both share and restrict the responsibility for a virtual host the DNS delegation model has proven useful.
In DNS delegation the owner of a domain can nominate a different name server--thus a different owner--to control the configuration of a set of sub domain resources.

In the DNS delegation model (and the Contour delegation model) the decision about when to delegate control occurs at configuration time.
At configuration time, the owner of the parent domain enters into their zone file the name servers that will respond to a specific subdomain.
At run time, when the name server hosting the parent domains' zone file receives a query, it consults its zone file and returns a record from its own database if present.
The contents of a DNS record do not change depending on the query, that is it say, there are no properties of a query that will return an A record in some cases and an NS record in others.

DNS delegation is fixed at configuration time.

## File inclusion as a model for delegation

Another way of looking at Contour's delegation model is akin to file inclusion.
Consider programming languages that support an `#include` or `import` syntax as part of prepossessing their source code.
As the language's compiler or interpreter is processing the source code and encounters an include style syntax it branches to the file being included and processes it as if it's contents were in line in the parent document.

Compare this to the way Contour builds its DAG.
For each root IngressRoute record contour consults each route.
If it encounters a `delegate:` key, Contour consults the contents to the delegate IngressRoute record and processes any routes, and delegates, therein.

Like DNS delegation, file inclusion is fixed at compilation time.
Once the inclusion statements have been processed there is no record that the result is the combination of several files.

## Delegation does not exist at run time

In both the analogies presented here, configuration, including delegation is applied once, before any requests are processed.
If requests generate different results--for example, if they are routed or respond differently based on the incoming request, the fact that the configuration chosen was part of a long chain of delegations, or if there was no delegation at all, is unrelated.

Processing delegation at configuration time and processing a request at run time appear independent.

## Delegation scopes

In the DNS model, delegation is implicitly scoped to a subdomain; if I delegate sub.example.com, then all the resources delegated are at sub.example.com or lower.
In the file inclusion model, the delegation scope is less obvious.
In most programming languages its only by convention that the file being included does not close any of the parents scopes.
A counter-example of this is SQL injection, the text being included can contain meta characters which close the parent's scope then creates another controlled by the attacker.
This is not possible in the DNS delegation model due to the implicit scope that surrounds the delegate.

When considering how delegation works in Contour we must consider a blend of both the DNS delegation and file inclusion models.
From the DNS delegation model we must take an idea of an enclosing scope; implicit restrictions which are overlaid on any configuration provided by the delegate.
