## kstatus support for HTTPProxy and ExtensionService

Clients that use [kstatus](https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus) can now check if `HTTPProxy` and `ExtensionService` are reconciled.
This includes [Helm](https://helm.sh/community/hips/hip-0022/), [Argo CD](https://argo-cd.readthedocs.io/en/stable/operator-manual/health/#using-kstatus), and [Flux](https://fluxcd.io/flux/components/kustomize/kustomizations/#health-checks).

For both resources, Contour now writes:
- `Stalled` in `.status.conditions`
- `observedGeneration` in `.status`

Downgrade note: older Contour releases do not support the `Stalled` status condition and cannot remove condition set by newer Contour version after a downgrade. This condition must be removed manually to prevent incorrect status reporting by kstatus clients.
