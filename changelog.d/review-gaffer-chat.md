- **Gaffer's chat reads sessions properly and keeps its tools in hand**: Gaffer reads a working agent's latest reply
  whole when the agent streams it in small pieces, and no longer fails to answer in a late stage of a long pipeline.
  When a model asks for several reet tools at once the first runs instead of the turn failing. A tool that ran while
  you cancelled the answer is still shown and audited. After Gaffer has read text other people wrote (a skill, the
  activity log), a tool that changes something asks first even on Allow. A conversation's size limit now counts tool
  requests.
