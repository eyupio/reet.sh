## STAGE-GUIDANCE-1: a stage's skills may add 6,000 bytes of guidance, not 2,000 (26 November 2026)

A person could not drop a library skill on any stage in the pathway editor: every stage said "too much guidance". The
skill alone formatted to more than the 2,000 bytes a stage's skills could add, so no stage could ever take it. The
owner chose to raise the limit to 6,000.

- `MaxStageGuidanceBytes` (`internal/gaffer/pathways.go`) is 6000. The template start (`templates.go`) and the
  **Use for this stage** route (`internal/api/skill_buddy.go`) held their own literal 2000 and now use the constant.
  The editor's copy is `MAX_STAGE_GUIDANCE_BYTES` in `web/src/lib/factory/pathways.ts`.
- Known cost: a stage's opening prompt is still 8,000 bytes (`MaxOutcomePromptBytes`). The guidance comes before the
  task brief, so a stage with near 6,000 bytes of guidance has its brief clipped hard, and with a long handover the
  guidance itself can be cut; the stage boundary is always kept (`stagePrompt` in `outcome_runs.go`).
- Not changed: a skill over 6,000 bytes still fits no stage, and the editor still words that as each stage having too
  much guidance. The 1,500-byte limits for composer drafts and system skills are about a chat message and stay.
