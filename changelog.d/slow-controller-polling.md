- **The Skills and Sync pages no longer pile up requests on a slow controller**: while a refresh was still waiting for
  its answer, every 15 seconds the page asked again. Each page now waits for its own refresh to finish, and an older
  answer that arrives late can no longer replace a newer one.
