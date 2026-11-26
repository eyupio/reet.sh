- **Factory pipelines stop and restart as you tell them**: after **Stop following**, Gaffer no longer starts following
  that session again by itself. Switching off a Gaffer provider's session text or code stays off while an outcome
  runs; the next stage is started but not followed, and says why. After a controller restart, pipelines that were
  starting wait up to two minutes for their machine to reconnect. An outcome stopped with **Stop starting sessions**
  can be archived or deleted once it shows as finished, an added pipeline always waits at Ready to merge, and a stage
  is no longer moved to its backup agent over a plan window that has already reset.
