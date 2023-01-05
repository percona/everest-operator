# Enhancement Idea Title

A table of contents is helpful for quickly jumping to sections of a KEP and for
highlighting any additional information provided beyond the standard KEP
template.

Ensure the TOC is wrapped with: `<!-- toc --><!-- /toc -->` tags, and then generate with `./update-toc.sh`.

<!-- toc -->
- [Enhancement Idea Title](#enhancement-idea-title)
  - [Release Signoff Checklist](#release-signoff-checklist)
  - [Open Questions \[optional\]](#open-questions-optional)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
      - [Story 1](#story-1)
      - [Story 2](#story-2)
      - [Examples](#examples)
        - [Alpha -\> Beta](#alpha---beta)
        - [Beta -\> GA](#beta---ga)
    - [Implementation Details/Notes/Constraints \[optional\]](#implementation-detailsnotesconstraints-optional)
    - [Risks and Mitigations](#risks-and-mitigations)
  - [Design Details](#design-details)
    - [Test Plan](#test-plan)
    - [Graduation Criteria](#graduation-criteria)
    - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
  - [Implementation History](#implementation-history)
  - [Drawbacks](#drawbacks)
  - [Alternatives](#alternatives)
  - [Infrastructure Needed \[optional\]](#infrastructure-needed-optional)
<!-- /toc -->

This is the title of the enhancement. Keep it simple and descriptive. A good title can help
communicate what the enhancement is and should be considered as part of any review.

To get started with this template:

1. **Make a copy of this template.** Copy this template into the main `proposals/NNN-Enhancement-Idea-Title` directory.
2. **Fill out the "overview" sections.** This includes the Summary and Motivation sections. These
   should be easy and explain why the community should desire this enhancement.
3. **Create a PR.** Assign it to folks with expertise in that domain to help sponsor the process.

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

The `Summary` section is incredibly important for producing high quality user-focused documentation
such as release notes or a development roadmap. It should be possible to collect this information
before implementation begins in order to avoid requiring implementors to split their attention
between writing release notes and implementing the feature itself.

A good summary is probably at least a paragraph in length.

## Motivation

This section is for explicitly listing the motivation, goals and non-goals of this proposal.
Describe why the change is important and the benefits to users.

### Goals

List the specific goals of the proposal and their measurable results. How will we know that this has succeeded?

### Non-Goals

What is out of scope for this proposal? Listing non-goals helps to focus discussion and make
progress.

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

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
