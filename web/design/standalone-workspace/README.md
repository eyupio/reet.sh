# Standalone Reet workspace

The approved design brings the public site's Fraunces and IBM Plex typography, ivory surfaces and orange accents into the installed application. Its entry screen identifies the current installation by its actual host. Anonymous visitors open sign-in or first-run owner setup; authenticated visitors open their existing workspace.

The interactive design reference is https://reet-private-workspace-design.jnnngs.chatgpt.site . It uses example data. The implementation retains the existing feature components and their real data.

## Feature contract

- Home keeps the existing Backlog, Active and Finished board, individual stage views, counts, sorting, compact density, combined filters, search and saved view preferences.
- Backlog keeps ideas, notes, saved prompts, image attachments, edit/delete actions, offline capture and the refine-before-start idea flow. Start failures retain their existing recovery and retry behaviour.
- The composer keeps machine, folder/project, agent, model, effort, worktree, base branch, Gaffer/chat, trust and YOLO options, including their existing capability and consent checks.
- The project rail keeps groups, ordering, selection, project scoping and launch handoffs. Needs-you cards retain their real approval/question controls.
- Sessions retain their conversation, terminal, workspace, actions, history, file context, layout modes and phone keyboard handling.
- Machines, skills, sync, setups, lists, teams, support, administration and account controls keep their existing routes and permissions. Search, bell, account menu, More and mobile bottom navigation remain available.
- Account, token, open-instance, owner setup, invitation, password reset and SSO paths keep their existing authentication logic.

## Website and application

The public showcase and documentation remain on reet.sh. The standalone application's entry and workspace routes return noindex metadata and headers, and do not advertise product canonical or social tags. Private app routes are excluded from the instance sitemap.

Explicitly published community content, lists and hosted skills retain their existing public pages and sharing behaviour. The instance sitemap continues to advertise that content. Documentation links in machine-readable discovery files point to the public documentation rather than nonexistent per-instance docs.

## Review

Review the existing board and authentication flows with their full fixture data, not only the abbreviated design reference. Test source is in web/tests; the board test includes saving an idea without a prompt, refining it, then starting it with the selected options.
