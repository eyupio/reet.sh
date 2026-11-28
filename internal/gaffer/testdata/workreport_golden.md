---
reet: work-report/1
outcome: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4
objective: Add CSV export to the reports page
state: running
---

## Pipeline: Export endpoint
state: done

- code: done (heavy)
- review: done

### Handover: code
Added GET /reports/{id}/export.csv with a streaming CSV writer. Reused the report's existing read permission check.

## Pipeline: Export performance
state: delivery review
stage in hand: test

- code: done (standard)
- test: in hand
- pr: pending

### Criteria
- [passed] An export of 10,000 rows finishes in under 5 seconds
- [unverified] A reader of another team's report is refused

### Waiting for the person
Delivery review.

To add more work to this outcome, answer with a work order whose header carries continues: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4.
