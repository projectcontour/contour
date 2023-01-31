This document outlines how we use GitHub.

## Milestones

Contour attempts to ship on a quarterly basis.
These releases are tracked with a milestone.
The _current_ release is the milestone with the closest delivery date.

Issues which are not assigned to the current milestone _should not be worked on_.

## Priorities

This project has three levels of priority:

- p0 - Must fix immediately.
This is reserved for bugs and security issues. A milestone cannot ship with open p0 issues.
- p1 - Should be done.
p1 issues assigned to a milestone _should_ be completed during that milestone.
- p2 - May be done.
p2 issues assigned to a milestone _may_ be completed during that milestone if time permits. 

Issues without a priority are _unprioritised_. Priority will be assigned by a PM or release manager during issue triage.

## Questions

We encourage support questions via issues.
Questions will be tagged `question` and are not assigned a milestone or a priority.

## Waiting for information

Any issue which lacks sufficient information for triage will be tagged `waiting-for-info`.
Issues with this tag may be closed after a reasonable length of time if further information is not forthcoming.

## Issue tagging

Issues without tags have not be triaged.

During issue triage, usually by a project member, release manager, or pm, one or more tags will be assigned.

- `Needs-Product` indicates the issue needs attention by a product owner or PM.
- `Needs-design-doc` indicates the issue requires a design document to be circulated.

These are blocking states, these labels must be resolved, either by PM or agreeing on a design. 

## Assigning an issue

Issues within a milestone _should_ be assigned to an owner when work commences on them.
Assigning an issue indicates that you are working on it.

Before you start to work on an issue you should assign yourself.
From that point onward you are responsible for the issue and you are expected to report timely status on the issue to anyone that asks.

If you cease work on an issue, even if incomplete, you should leave a comment to that effect on the issue and remove yourself as the assignee.
From that point onward you are no longer responsible for the issue, however you may be approached as a subject matter expert--as the last person to touch the issue--by future assignees.

For infrequent contributors who are not members of the Contour project, assign yourself by leaving a comment to that effect on the issue.

*Do not hoard issues, you won't enjoy it*

## Requesting a review

PRs which are related to issues in the current milestone should be assigned to the current milestone.
This is an indicator to reviewers that the PR is ready for review and should be reviewed in the current milestone.
Occasionally PRs may be assigned to the next milestone indicating they are for review at the start of the next development cycle.

All PRs should reference the issue they relate to either by one of the following;

- `Fixes #NNNN` indicating that merging this issue will fix issue #NNNN
- `Updates #NNNN` indicating that merging this issue will progress issue #NNNN to some degree. 

If there is no `Updates` or `Fixes` line in the PR the review will, with the exception of trivial or self evident fixes, be deferred.

[Further reading][1]

## Help wanted and good first issues

The `help wanted` and `good first issue` tags _may_ be assigned to issues _in the current milestone_.
To limit the amount of work in progress, `help wanted` and `good first issue` should not be used for issues outside the current milestone.

[1]: https://dave.cheney.net/2019/02/18/talk-then-code