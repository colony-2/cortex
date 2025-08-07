# Project: vibethis

## Description
vibethis is a react ui + golang backend that allows users to create and manage various components called "boxes". Each box is contained in a separate directory. Key directories:
- server: is the golang server
- web: react frontend
- example: an example set of artifacts that can be presented

## Patterns to Follow
- Always create tests to validate any change you make.
- *never* weaken tests to get them to pass.
- If you think a test is no longer valid, ask for confirmation before removal, explaining why it is invalid.
- Always request guidance before switching implementation direction (main code AND tests)
- must build & test after making changes to confirm they work.
- You should commit your changes frequently
- Always use moon to configure build and test commands.



