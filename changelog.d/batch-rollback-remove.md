- **Roll back or remove OpenCode on several machines at once**: in Fleet software, under Machines, **What to do** now
  offers **Roll back** and **Remove reet's copy** beside the reviewed update. Each machine row says whether it can and
  why not: only a copy reet installed is changed, a rollback needs the earlier version reet kept, and OpenCode installed
  outside reet is never touched. Both ask you to type `rollback` or `remove` before they start, and the controller
  refuses a batch without that word. They run one machine at a time and stop at the first failure, like an update.
