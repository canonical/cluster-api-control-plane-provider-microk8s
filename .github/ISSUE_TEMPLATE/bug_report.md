---
name: Bug Report
about: Something is not working
---

<!--
   Thank you for submitting an issue. Please fill in the template below
   information about the bug you encountered.
-->

#### Summary
<!-- Please explain the bug in a few short sentences -->

#### What Should Happen Instead?
<!-- Please explain what the expected behavior is -->

#### Reproduction Steps
<!-- Are you able to consistently reproduce the issue? Please add a list of steps that lead to the bug. -->

1. ...
2. ...

#### Environment:
<!-- Please provide the following information: -->
- Control plane provider version:
- Bootstrap provider version:
- Infrastructure provider: <!-- (e.g. AWS, GCP, Azure, etc.) -->
- Infrastructure provider version:
- MicroK8s version:
- If upgrading:
   - From version:
   - To version:
   - Upgrade Strategy: <!-- (e.g. InPlaceUpgrade) -->

#### Logs and Inspection Report
<!-- Please provide the following logs and inspection report: -->
<!-- For the logs, add a pastebin link or attach the logs as a file. -->
<!-- For the inspection report, run `microk8s inspect` on the workload cluster nodes and attach the output. -->
- Control plane provider logs:
- Bootstrap provider logs:
- Infrastructure provider logs:
- MicroK8s inspection report:
   - If upgrading, please attach the inspection report from both versions.

#### Can you suggest a fix?
<!-- (This section is optional). How do you propose that the issue be fixed? -->

#### Are you interested in contributing with a fix?
<!-- yes/no, or @mention maintainers. Community contributions are welcome. -->

<!-- Thank you for making MicroK8s CAPI providers better -->
