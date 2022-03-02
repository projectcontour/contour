---
title: How We Work
layout: how-we-work
---

This page captures how we work on Contour.

## Github Issue Management

- When you pick up an issue, assign it to yourself.
When you stop working, unassign yourself.
While you hold an issue, you are responsible for giving status reports on it; over communicate, don’t let others speak for you, or worse, guess.

- Don't work on an issue assigned to someone else. If you think they're over committed  or stuck, please ask.

- Don't assign an issue to someone else without talking to them directly.

- Hoarding issues is not saving for a rainy day, you can only work on one thing at a time, you should avoid holding more than one issue at a time.

- Log an issue or it didn’t happen. 

- When submitting a PR add the appropriate release milestone and also add the appropriate Github "release-note" label if this PR warrants getting called out in the next release.

## Code reviews

- Everyone is responsible for code reviews.
If someone asks you for a review, or they use GitHub for the same, you should aim to review it (timezones permitting) promptly.
Your aim is to give feedback, not land the as soon as you are asked to review it.

- The smaller the change, the better the PR process works.
Everything flows from this statement.

- [Talk about what you intend to do, then do the thing you talked about.][1]
GitHub review tools suck for extended debate, if you find you’re taking past your reviewer, its a sign that more design is needed.

## Coding Practices

- Before you fix a bug, write a test to show you fixed it.

- Before you add a feature, write a test so someone else doesn’t break your feature by accident.

- You are permitted to refactor as much as you like to achieve these goals.
As Kent beck said, ["make the change easy, then make the easy change."][2]

## Github Labels

Github issues should be triaged and have their status recorded.
Triaging issues and labeling them appropriately makes it easy for the issue submitter and other contributors to see what the state of an issue is at any time.
The goal of triaging an issue is to make a decision about what should be done with it.
The issue should be investigated enough to fully understand and document the problem and then decide whether the issue should be addressed by the project.
It may not be necessary to decide how an issue should be addressed during triage, since that could involve substantial research and design.

- To be considered triaged, an issue must have the "lifecycle/accepted" label.
- If you are in the process of investigating an issue, assign it to yourself and apply the "lifecycle/investigating" label.
- In almost all cases, triaged issues should have kind and area labels. 
- The priority and size labels are informative. Contributors should apply them at their discretion.


[1]: https://dave.cheney.net/2019/02/18/talk-then-code
[2]: https://twitter.com/kentbeck/status/250733358307500032?lang=en
