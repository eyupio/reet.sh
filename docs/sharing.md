# Personal, shared and public collections

Reet keeps your personal shelf separate from lists. Adding a skill to a list does not publish your personal notes, tags, likes, usage history or machine inventories. **Add all to my shelf** copies the list's skill references into your own shelf; it does not enrol you in another person's machine sync.

## Find the right collection

- **Personal shelf** contains your own skills and private notes.
- **Lists → My lists** shows collections you own, with an audience badge on each.
- **Lists → Shared with me** shows lists shared directly with your account.
- **Lists → Teams & organisations** shows accessible group collections.
- **Community** is a public directory of deliberately published lists and their unique skills. It can be browsed without signing in. It covers this reet site, not a federated search across other installations.

New lists default to **Personal**. The creation form and list settings include an audience preview before you save. Signed-in writers can save a community skill directly to their personal shelf, or open a public list in their account to add all its skills.

| Audience | Who can read | Discovery |
| --- | --- | --- |
| Personal | The list owner | My lists |
| Selected people | Individually invited accounts, minus exclusions | Their shared lists |
| Team or organisation | Current group members plus individually invited accounts, minus exclusions | Their group lists |
| Public community | Anyone, including signed-out visitors | Community lists and skills, plus a public URL |
| Unlisted link | Anyone who has the URL | Hidden from community discovery |
| Legacy instance audience | Signed-in accounts on this server | Legacy audience tab |

Instance owners and admins retain administrative access, including to personal/private lists. They do not count as invited participants merely because they are administrators. An account's viewer role stays read-only even when a group or individual grant asks for editor access.

The legacy audience preserves existing `members` lists. It is not renamed or migrated into public access, and it is offered only when editing an existing list using it. Existing private and link lists also retain their current access. Creating a new list no longer defaults to everyone signed in.

## Teams and organisations

Open **Teams** to create an independent team or an organisation, optionally with teams inside it. Add existing account emails as members or managers. Group owners/managers can rename groups and manage members; owners appoint managers and delete groups. Read-only accounts cannot create or administer groups. Invitations to create new reet accounts remain under Admin.

Each group has explicit membership: joining an organisation does not automatically join every team, and joining a team does not enrol someone in its organisation's audience. An organisation list is shared with that organisation's members; a team list is shared with that team's members. Organisations organise teams within one reet server; they do not establish separate database tenants, SSO directories or independent instance administrators.

A group's owner or managers choose what that group is emailed about under its **Notifications** details: people joining, leaving or changing role, and lists shared with the group. Notices go to the owner and managers by default, or to every member, and any extra addresses the managers add (who receive the email but no access). The instance owner sets the mail server under **Admin → Email**; see [email](email.md).

A group's owner and managers can also read and answer support tickets opened for the group. Other members cannot see them unless they opened the ticket. See [support](support.md).

For a list, choose **Team or organisation**, select a group, then choose **View only** or **Edit skills and notes** for inherited access. Add named collaborators outside the group when needed. New group members automatically gain inherited list access; removed members lose it immediately. A separate direct invitation continues to grant access until it is removed or excluded.

Use **People & exceptions** to exclude individual account emails. Exclusions override both group and direct grants for ordinary accounts, so a directly invited editor who is excluded cannot read, edit, discover the list or see its name attached to a skill card. Owners/admins retain administrative access. Read access cannot be excluded from public or unlisted audiences; choose a private audience if you need that restriction.

Deleting a group revokes its inherited grants and retains its lists, skills and direct invitations. Delete child teams before deleting their organisation. Lists whose group was deleted remain accessible to their owner and directly invited accounts; choose another group in list settings to restore inherited access.

## Publishing and revoking publication

Choose **Public community** and use **Publish to public community** to publish a list. The list name, description, skill metadata and **list notes** become public. Review the list notes first. Local-only skills must be linked to an install source before publication. The public directory does not show collaborator names/emails, group membership, exclusions, account IDs, personal notes, personal adoption counts or machines.

Public publication is separate from unlisted links. The existing owner-controlled **link sharing** switch still controls unlisted URLs; it does not turn a list public or list it in the community. Only explicit public publication does that.

Changing a public list back to a private audience removes it from discovery and makes its public URL unavailable. People who already read or copied published content may retain their own copies. Deleting a list also removes its public projection. Public responses use `Cache-Control: no-store`; private list metadata is checked against the same audience rules as list contents.

## API additions

- `GET /api/community` is anonymous and returns only published list summaries and their public skill metadata.
- `GET /api/v2/public/lists/{slug}` reads a public or enabled unlisted list without exposing people or personal shelf state.
- `GET/POST /api/v2/groups`, `PATCH /api/v2/groups/{id}` and `POST /api/v2/groups/{id}/members` manage caller-accessible groups and membership.
- Create/update list requests accept `audience: { groupId, groupRole, people: [{email, role}], excluded: [email] }`. List audience and settings changes are committed together. Only list owners/admins may change them.

Existing scanner, sync target IDs, shelves and list member endpoints remain compatible. Personal lists reject direct collaborator additions until the owner changes their audience to Selected people or Team/organisation.
