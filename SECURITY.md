# Security Policy

## Supported versions

RePlan is pre-1.0. Only the latest code on `main`, or the latest release when
one exists, receives security fixes. Older commits, snapshots and releases are
not supported.

## Reporting a vulnerability

Report vulnerabilities only through GitHub private vulnerability reporting:

1. Open this repository on GitHub.
2. Select **Security**.
3. Select **Report a vulnerability**.

Do not open a public GitHub issue, discussion or pull request for an
unresolved vulnerability. The project does not publish an e-mail address for
security reports.

## What to include

Please include enough detail to reproduce and assess the problem:

- the affected component and version or commit;
- the expected and observed behavior;
- clear reproduction steps or a minimal proof of concept;
- the security impact and any known preconditions;
- suggested mitigations, if you have them;
- whether you want public credit.

Keep the report focused on the vulnerability. Remove private project data and
credentials from logs, fixtures and screenshots.

## What to expect

This project has one maintainer and handles reports on a best-effort basis. No
response or resolution time is promised.

A reporter can expect, in order:

1. acknowledgement of the report;
2. assessment of its impact and scope;
3. a fix or a decision explaining why no fix will be made;
4. coordinated disclosure and credit, if the reporter wants them.

Please keep the report private until disclosure is coordinated.

## Scope

This policy covers security problems in:

- the `replan` CLI;
- helper scripts under `scripts/`;
- the RePlan agent kit;
- the OpenProject plugins under `integrations/openproject/plugins/`.

OpenProject itself and other upstream or third-party software are out of scope.
Report vulnerabilities in those projects through their own security channels.
If a RePlan integration exposes an upstream problem, the maintainer may help
identify the correct boundary, but cannot coordinate for the upstream project.

## Secrets and credentials

If a report involves a leaked token, password or other credential, say where
it appeared and what system it could access. Do not paste the credential value
into the report. Revoke or rotate it through the system that issued it as soon
as possible.
