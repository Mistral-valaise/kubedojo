# Cloud Native Application Delivery

**16% of the exam.** How a change travels from a commit into a cluster, and how to look at it once it
runs. The questions ask for vocabulary and for the difference between neighbouring terms, not for
commands typed from memory.

## The 20% that solves 80%

### CI and the two CDs

| Term | Automated up to | Ends with |
|---|---|---|
| Continuous integration (CI) | merge, build, test | a tested artifact, for example a container image |
| Continuous delivery (CD) | deployment into an acceptance environment | a release that is ready for production |
| Continuous deployment (CD) | deployment into production | the change is live, no manual gate |

CI is the precondition for both CDs. A pipeline chains the stages; what separates the terms is where
the automation stops.

### GitOps

GitOps applies the Kubernetes idea of desired state to delivery itself. The OpenGitOps principles:

1. **Declarative**: the desired state is described, not scripted.
2. **Versioned and immutable**: it is stored with a complete history, typically in Git.
3. **Pulled automatically**: software agents fetch the desired state from the source.
4. **Continuously reconciled**: the agents compare actual and desired state all the time and correct
   the difference.

| | Push (classic pipeline) | Pull (GitOps) |
|---|---|---|
| Who applies | the CI/CD system, from outside | an agent inside the cluster |
| Credentials for the cluster | held by the pipeline | stay in the cluster |
| Manual change in the cluster | stays until the next run | is detected as drift and reverted |
| Rollback | run an older pipeline | revert the commit |

Argo CD (part of the Argo project) and Flux are graduated CNCF projects built for the pull side.

### Helm and Kustomize

Both produce the manifests for one application in several variants. They do it differently.

| | Helm | Kustomize |
|---|---|---|
| Idea | package manager | customisation without templates |
| Unit | **chart**: templates plus default values | **base**: plain manifests plus `kustomization.yaml` |
| Variant | your own **values** at install time | an **overlay** that refers to bases and patches them |
| In kubectl | no, separate `helm` CLI | yes: `kubectl apply -k`, `kubectl kustomize` |
| Tracks what it installed | yes | no |

Helm's three nouns: a **chart** is the package, a **repository** is where charts are collected and
shared, a **release** is one installed instance of a chart in a cluster. The same chart installed
twice gives two releases. Every upgrade or rollback of a release adds a **revision**, which is what
`helm rollback` goes back to.

In Kustomize a base knows nothing about its overlays and can serve many of them, for example one per
environment. Generators create ConfigMaps and Secrets from files with a content hash in the name, so a
changed configuration rolls the Pods.

### Rollout strategies

| Strategy | How the new version arrives | Old and new together | Price |
|---|---|---|---|
| Recreate | all old Pods stop, then new ones start | never | downtime |
| Rolling update | Pods are replaced step by step | during the rollout | both versions must be compatible |
| Blue-green | a second complete environment is built and tested, then all traffic switches at once | both exist, one serves | double resources |
| Canary | a small share of live traffic goes to the new version and grows | yes, by intent | needs traffic control and monitoring |

A Deployment knows only `RollingUpdate` (the default) and `Recreate`. `maxSurge` says how many Pods
may exist above the desired number during the update, `maxUnavailable` how many may be missing.
`kubectl rollout undo` returns to the previous revision. Blue-green and canary are patterns built on
top: two Deployments and a Service selector or replica ratio, or weighted routes in Gateway API, a
service mesh, Argo Rollouts or Flagger.

### Debugging what was delivered

| Command | Shows | Needs |
|---|---|---|
| `kubectl get` | status at a glance: READY, STATUS, RESTARTS | the object |
| `kubectl describe` | specification, container states and the **events** | the object |
| `kubectl logs` | stdout and stderr of a container; `--previous` for the instance before the restart | a container that has started once |
| `kubectl exec` | runs a program **from the container's image** inside the container | a running container with that program |
| `kubectl debug` | adds an ephemeral container, copies a Pod, or opens a Pod on a node | permission for it |

An **ephemeral container** is a temporary container added to a running Pod for troubleshooting. It
brings its own image, and with it the tools that a minimal or distroless application image lacks. It
has no ports and no probes, is never restarted, and cannot be removed from the Pod again. With
`--target` it shares the process namespace of an application container.

`kubectl debug` in its three forms: `kubectl debug -it <pod> --image=busybox --target=<container>`
for the running Pod, `--copy-to=<name>` for a modified copy of the Pod (other image, other command),
and `kubectl debug node/<node> -it --image=busybox` for a Pod on the node with the host filesystem
mounted.

## Common confusions

- Continuous delivery and continuous deployment share an abbreviation. Delivery makes every change
  releasable, deployment releases every change.
- GitOps is not "manifests are in Git". Without an agent that pulls and reconciles it is a pipeline
  with a repository.
- A chart is not a release, and a release is not a revision.
- Rolling update is a Deployment strategy; blue-green and canary are not values of
  `spec.strategy.type`.
- Events come from Kubernetes (scheduled, pulled, killed), logs come from the application. A Pod that
  never started has events and no logs.
- `kubectl exec` enters an existing container; `kubectl debug` adds a new one.
