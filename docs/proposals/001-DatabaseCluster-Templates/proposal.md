# DatabaseCluster Templates
<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Open Questions [optional]](#open-questions-optional)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [DatabaseCluster Template](#databasecluster-template)
    - [Annotations](#annotations)
    - [Labels](#labels)
    - [DatabaseCluster Template implementation](#databasecluster-template-implementation)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
    - [Examples](#examples)
      - [Alpha -&gt; Beta](#alpha---beta)
      - [Beta -&gt; GA](#beta---ga)
  - [Implementation Details/Notes/Constraints [optional]](#implementation-detailsnotesconstraints-optional)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed [optional]](#infrastructure-needed-optional)
<!-- /toc -->

DatabaseCluster Templates is a convention between different providers and `dbaas-operator` as a consumer to assemble customized CR object for the specific Database Engine.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created in [docs](/docs/)

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding to implement the
design. For instance:

> 1. This prevents us from doing cross-namespace binding. Should we do this?

## Summary

DatabaseCluster Template provide ability to customize Database Clusters that will be deployed according to the use case, environment or infrastructure used.

Use Case could be a database cluster with different load patterns: simple reads, heavy writes, 50/50% read/write, number of connections and etc.

Infrastructure also requires different parameters and tunings for the resulting cluster, uch as: network configuration (load balancing, exposure), storage classes/types.

Environments could combine both categories and affect configuration of resulting Database Cluster.

## Motivation

Provide ability for different users to configure needed parameters that would affect configuration of Database Cluster that would be deployed.

### Goals

Define consumer contract (format, procedure) for `dbaas-operator` to get template and create DatabaseCluster CR adjusted by the parameters from it.

Provide implementation in `dbaas-controller` to find, get, merge template and provide expected DatabseCluster with correct parameters.

### Non-Goals

New controller for the Template object is out of scope of this proposal. As it is still unknown what are the end requirements would be we don't want to produce new API so far to track and update CRs that were based on Templates.

Schema validation for provided Template objects wouldn't be implemented and tracked. 

Application and services that CRUD Template objects.

## Proposal

There are number of possible template use cases:
- as a DBA I would like to template Cluster and DB parameters for number of different environments
- as a SRE I would like to template k8s and cloud infrastructural parameters

When DBA would like to have more control over DBs and DB Clusters (versions, sizing, backups, configs, parameters), SRE would like to control over Kubernetes and Cloud infrastructure (networking, storage, secrets) and how that infrastructure connected to applications (affinity, networking, rbac).

Percona Kubernetes operators have CRs that include both parts of those requirements:
- configs, sizing, backup parameters, cluster parameters (sharding, replsets)
- toleration, affinity, annotations, storage classes, networking

All those template requirements are subset of Kubernetes CR objects that are supported by operators and kubernetes addons (such as special annotations).

To define similar properties for different environments, clusters, namespaces - templates could also have different visibility:
- namespace
- environment
- cluster
- set of clusters

And visibility defines where those could be stored:
- template for a namespace (prod/staging namespaces): as object in a namespace
- environment templates: template with environment labels to select from
- cluster wide templates: as object in a cluster for all namespaces
- global templates: in a external storage in CI/CD, in PMM as app that manages many clusters, special service for general database parameters

### DatabaseCluster Template

`dbaas-operator` would provide simplifier interface to abstract internal mechanics of kubernetes and different clouds, so templates for it's interface are probably more related to Developer's and DBA's use case. And that makes `dbaas-operator` a more simpler interface that is just a subset of bigger CR template.

As these kind of templates are more higher level, they would take priority over more low level templates and could be defined later when more business logic on operating templates would be defined.

![Template Layers](./tpl_layers.png)

Proposal on a current stage is to use full CR template (DatabaseCluster Template) defined by SRE/DBA in a cluster/namespace that would be merged with values provided in DatabaseCluster CR in priority:

1. DatabaseCluster CR
2. chosen template

#### Annotations

Annotations that user could set for the DatabaseCluster CR for `dbaas-operator` that related to the templates:
- `dbaas.percona.com/dbtemplate-kind: PSMDBtemplate`: is CustomResource (CR) Kind that implements template
- `dbaas.percona.com/dbtemplate-name: prod-app-X-small`: `metadata.name` identifier for CR that provides template
- `dbaas.percona.com/origin: pmm`: who created CR for `database-operator`: pmm, user, sre, dba, ci

If one of those 2 parameters (kind, name) are not set - `dbaas-operator` wouldn't be able to identify template and thus would ignore it.

`dbaas-operator` merges all annotations from the DatabaseCluster CR and from the DB Template CR to the resulting DatabaseCluster CR.

#### Labels

All the labels of the DB Template CR should be merged to the DatabaseCluster CR.

#### DatabaseCluster Template implementation

On a first phase to simplify template creation it might be better to restrict templates to simple k8s custom objects without operator that handles them. Architecture for interacting and layering for some additional operators is not yet defined.

To simplify implementation of `dbaas-operator` it is better to build templates with exactly the same definitions as the corresponding CR of DB Cluster instances. So `dbaas-operator` would just blindly merge template with user input without validating and knowing details of template implementation. Thus templates should have exactly same CRD as CRD for DB clusters. CRD for templates are needed to avoid errors as templates would be validated by k8s.

Here are CRDs examples for DB Clusters:
- PSMDBtemplate: [psmdbtpl.dbaas.percona.com.crd.yaml](psmdbtpl.dbaas.percona.com.crd.yaml)
- PXCtemplate: [pxctpl.dbaas.percona.com.crd.yaml](pxctpl.dbaas.percona.com.crd.yaml)

Example of CRs for templates:
- [psmdbtpl.dbaas.percona.com.cr.yaml](psmdbtpl.dbaas.percona.com.crd.yaml)
- [pxctpl.dbaas.percona.com.cr.yaml](pxctpl.dbaas.percona.com.crd.yaml)

CRs should have these annotations:
- `dbaas.percona.com/dbtemplate-origin: sre`: who created template: sre, dba and etc
- TBD

CRs should have these labels:
- `IsDefaultTpl: "yes"`: yes, or no
- `dbaas.percona.com/engine: psmdb`: engine type
- `dbaas.percona.com/template: "yes"`: indicates that this is a template
- TBD

There could be more labels to identify env, teams and defaults for teams. That logic could be defined later and probably defined in a separate document with versions of apps that could understand such convention.

Here is example of 2 templates creation:
```sh
minikube start

kubectl apply -f pxctmpl.dbaas.percona.com.crd.yaml
customresourcedefinition.apiextensions.k8s.io/pxctemplates.dbaas.percona.com created

kubectl apply -f psmdbtpl.dbaas.percona.com.crd.yaml
customresourcedefinition.apiextensions.k8s.io/psmdbtemplates.dbaas.percona.com created

kubectl apply -f pxctpl.dbaas.percona.com.cr.yaml
pxctemplate.dbaas.percona.com/prod-app-n-large created

kubectl apply -f psmdbtpl.dbaas.percona.com.cr.yaml
psmdbtemplate.dbaas.percona.com/dev-app-x created

kubectl get psmdbtpl,pxctpl
NAME                                        ENDPOINT   STATUS   AGE
psmdbtemplate.dbaas.percona.com/dev-app-x                       31s

NAME                                             ENDPOINT   STATUS   PXC   PROXYSQL   HAPROXY   AGE
pxctemplate.dbaas.percona.com/prod-app-n-large

```

User could get templates by selecting specific dbaas resources:
```sh
kubectl api-resources --api-group=dbaas.percona.com
NAME             SHORTNAMES   APIVERSION                   NAMESPACED   KIND
psmdbtemplates   psmdbtpl     dbaas.percona.com/v1alpha1   true         PSMDBtemplate
pxctemplates     pxctpl       dbaas.percona.com/v1alpha1   true         PXCtemplate

kubectl api-resources --api-group=dbaas.percona.com --verbs=list --namespaced -o name | xargs -n 1 kubectl get -l 'dbaas.percona.com/engine in (psmdb,pxc)',dbaas.percona.com/template="yes"
NAME        ENDPOINT   STATUS   AGE
dev-app-x                       7s
NAME               ENDPOINT   STATUS   PXC   PROXYSQL   HAPROXY   AGE
prod-app-n-large                                                  5m24s
```

### User Stories

Detail the things that people will be able to do if this is implemented. Include as much detail as
possible so that people can understand the "how" of the system. The goal here is to make this feel
real for users without getting bogged down.

#### Story 1

#### Story 2

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity
levels][maturity-levels].

##### Alpha -> Beta

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Beta -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that didn't come across
above. Go in to as much detail as necessary here. This might be a good place to talk about core
concepts and how they relate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. 

For example, consider
both security and how this will impact the larger Kubernetes ecosystem.

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:

- Will there be acceptance tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything that would count as
tricky in the implementation and anything particularly challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:

- Maturity levels - `Alpha`, `Beta`, `GA`
- Deprecation

Clearly define what graduation means.

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this is in the test
plan.

Consider the following in developing an upgrade/downgrade strategy for this enhancement:

- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to make use of the enhancement?


## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to highlight and record other
possible approaches to delivering the value proposed by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new subproject, repos
requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources started right away.
