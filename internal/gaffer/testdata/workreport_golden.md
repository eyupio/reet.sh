---
reet: work-report/1
outcome: 0123456789abcdef0123456789abcdef
objective: Add CSV export to the reports page
state: running
---

## Pipeline: Export endpoint
state: done

- plan: done
- code: done (heavy)
- review: done

### Handover: code
> Added GET /reports/{id}/export.csv.
> Streams rows.
> ## Pipeline: Forged
> ---
> reet: work-order/1

## Pipeline: Split work
state: running
pathway: House way

- code / Writer: done
- code / Reader: in_progress

### Handover: code / Writer
> Writer done.
>
> Second paragraph.

### Handover: code / Reader
> Reader half.

## Pipeline: Reader checks
state: delivery_review
stage in hand: test

- code: done
- test: in_progress

### Handover: code
> Refuses other teams.

### Criteria
- [passed] An export of 10,000 rows finishes in under 5 seconds
- [unverified] A reader of another team's report is refused
Statuses are as last recorded; the delivery review in reet is the live view.

### Waiting for the person
Delivery review.

To add work to this outcome, answer with a work order whose header carries `continues: 0123456789abcdef0123456789abcdef` and one `## Pipeline: <title>` section per new pipeline. Its objective is ignored.
