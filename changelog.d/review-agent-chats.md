- **Agent chats recover better**: a chat whose agent has stopped responding no longer hangs Stop and End; reet ends it
  after 15 seconds. After reet restarts on a machine a chat comes back in the mode you chose, and after a controller
  restart a finished chat still shows as finished. A message the agent could not take is refused or said in the chat,
  never dropped, and a queued message is shown once, when it is sent. Notices name the chat's own agent. The
  controller no longer keeps every ended chat's transcript in memory. The Start an agent dialog is locked while it
  checks where a session will start.
