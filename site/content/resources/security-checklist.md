---
title: Security Response Checklist
layout: page
---

This document outlines a checklist for Contour security team members (at time of writing, this is the same people as the maintainer team) to step through in the event Contour has a CVE that needs to be mitigated.

## A CVE has been reported, what do I do?

1. User discovers a vulnerability and notifies cncf-contour-maintainers@lists.cncf.io
1. Contour maintainer team triages the vulnerability with the reporter and decide patch releases (multiple minors could be impacted) as well as downstream distributors.
1. Create a Security Advisory Draft on github Contour repo https://github.com/projectcontour/contour/security/advisories
    - Requires patched versions 
    - As part of this, fill out the CVSS score and CWE enumerator, and request a CVE ID via Github.
1. Create a private fork for the Security Advisory using the Advisory page, and ensure everyone who needs to can see it.
1. Do not publish draft, keeping it in draft mode until we release patch
    - Remember to give credit to the reporter, they can however remain anonymous or keep their company info private if they wish
1. Communicate to the reporter that draft is created & awaiting for precise dates for releases
1. Send email to the Distributors (cncf-contour-distributors-announce@lists.cncf.io) mailing list on disclosure and patch releases dates, can include
    - Learn from previous mistakes, send this through the web interface at https://lists.cncf.io/g/cncf-contour-distributors-announce/ !
      Don't use a client that may "correct" the address to another one for you.
    - Description of vulnerability
    - Contour versions affected
    - Known attack vectors
    - Possible workarounds
    - Next step including patch releases
    - Leave out the CVE ID
    - Get buy-in from the distributors on release date, or at least see if there are objections
    - Post the Embargo note (sourced from https://projectcontour.io/resources/security-process/) at the bottom
      ```
      The information that members receive on the Contour Distributors mailing list must not be made public, shared, or even hinted at anywhere beyond those who need to know within your specific team, unless you receive explicit approval to do so from the Contour Security Team. This remains true until the public disclosure date/time agreed upon by the list. Members of the list and others cannot use the information for any reason other than to get the issue fixed for your respective distribution's users.
      Before you share any information from the list with members of your team who are required to fix the issue, these team members must agree to the same terms, and only be provided with information on a need-to-know basis.

      In the unfortunate event that you share information beyond what is permitted by this policy, you must urgently inform the [Contour Security Team](https://projectcontour.io/resources/security-process#mailing-lists) of exactly what information was leaked and to whom. If you continue to leak information and break the policy outlined here, you will be permanently removed from the list.
      ```
    - Add #security tag to message
1. Release patches for all supported minors
    - Submit PRs for fixes with pithy commit messages, or even no commit message.
      The point is to ensure that we don't give away the CVE before the public release in a commit message.
1. When all patches are released and the embargo date is reached, publish the security advisory which was in draft mode.
1. Can now send above email to the broader public Contour users mailing list as well
1. Follow up on cncf-contour-distributors-announce@lists.cncf.io as well notifying users that releases are out
1. Do a team retrospective on the release for the CVE if applicable

