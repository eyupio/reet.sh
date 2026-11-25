- **Limits on imports and skill notes**: a single skills import now holds at most 16,384 entries and answers 413 with
  a message asking you to send it in parts, instead of holding every other import and machine merge while it runs. A
  skill name over 255 bytes is skipped and listed among the import's errors. A skill's note is at most 20,000
  characters, the most the MCP tools already took, and a longer one is refused with a message.
