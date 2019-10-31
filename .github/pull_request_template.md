### Description

<!--
A description of the change in this PR.
Place a reference to the Github issue or the PagerDuty incident, if any, here.
-->

### Type of Change

- [X] Routine: low risk change
- [ ] Declared: a significant or high risk change
- [ ] Emergency: a change made to fix an on-going incident

<!--
Classify this change as one of the following, according to the change management
policy in https://drive.google.com/drive/folders/1YgEceW5HQ2bQ4OAuhFIgN1oHmLk9RfTV
-->

### Testing

- [ ] None: Documentation change
- [ ] Unit Tests
- [ ] Functional Tests
- [X] Manual Tests: <!-- Describe how this should be tested -->

<!--
Describe the testing procedure that has been done, or will be done, if any.
Optional for routine changes, mandatory for declared changes.
-->

### Rollout Procedure

- [ ] None: Non-configuration management change
- [ ] Manual Roll-Out: <!-- If checked, replace with description -->
- [X] Automated Roll-Out (Full): Changes will be applied when an image is built from master and pushed to redcanary/cb-event-forwarder:latest

<!--
Describe how this change will be rolled out. For most changes in this repository,
the following default should suffice.
-->

### Rollback Procedure

Reverting the merge of this PR, and applying the rollout procedure above, will roll
back the changes in this PR.

<!--
Describe how this change will be rolled back. For most changes in this repository,
the following default should suffice.
-->

### Approvals Needed

Routine Changes:
 - @redcanaryco/team-sre or @redcanaryco/team-processing (Optional)

Declared Changes:
 - @redcanaryco/team-sre and @redcanaryco/team-processing (Required Prior to Merge)

Emergency Changes:
 - @redcanaryco/team-sre and @redcanaryco/team-processing (Required Prior/Post Merge)
