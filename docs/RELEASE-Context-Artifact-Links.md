# Markdown links open attached artifacts in the current viewer

Relative file links in rendered Markdown now select the matching attachment in the same review reader or job node artifact modal. Nested paths resolve relative to the current artifact. Review IDs and unique display names can also identify review documents.

Missing, ambiguous, or invalid local targets show a message inside the reader. Local links and heading fragments stay in the viewer; they do not open new tabs or create browser history entries. External and root-relative links retain their existing behavior. Downloads and Markdown source remain available through the viewer controls.

Only attachments already available to the current review or opening job node can be selected. No stored Markdown, producer metadata, or API format needs migration.
