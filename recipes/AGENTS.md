## Purpose
You are an agent responsible for implementing and maintaining recipes for the c2 software development system.

## C2 Concepts
c2 is used for completing tasks related to building and running software. c2 can be used to build anything from simple tools (a bash script) to complex cloud-run multi-component, multi-tenant services. c2 is responsible for creating, validating, debugging software. For projects that need to be run in a cloud provider, C2 is also responsible for , provisioning infrastructure, deploying code, upgrading software and systems and monitoring using tools like terraform and hyperscalers like AWS.

c2 is designed to be an adaptive, extensible system that is built using the following primitives:

- actor: any entity working to move a c2 project forward. an entity may be an llm-based agent, a human or some kind of external process.
- project: a specific software initiative. May be an operationally complex SaaS service or a simple CLI tool. Each project is associated with a specific git repository (subcomponents, what c2 may be in other repositories).
- cell: a context for a specific task. Cells are designed to decompose a software project into distinct, manageable chunks/components.
- recipe: a recipe is a c2 workflow that will be completed in a durable way. a recipe is composed of ops (operations), sequences and state machines. Recipes are versioned in c2 and can be saved or saved and published. When executing recipes, the published version of recipes are used. See guides/RECIPE_AUTHORING_GUIDE.md for more details
- op: an op or operation is the building block of a recipe. Ops are combined together to achieve arbitrary goals. Ops include things like consulting an llm, running a bash command, merging some git code into a repo, starting and/or consuming execution of other recipes and having a coding agent build some code. Ops defined input and output structured types. See guides/ops/* for information about available ops.
- template: recipes support the use of CEL-based template expansion and referencing of both global context and specific op/state/sequence outputs in a recipe. (see )
- job: a job is a specific execution of a recipe. Jobs can be triggered by tickets, other jobs or directly via the cli. The words job and workflow are often used interchangeably in c2. CLI direct execution is usually used for job authoring and debugging.
- job story: a job story is the description of what happened in a job. it allows introspection of a job's execution both while it is running and upon completion. A story describes the sequence of steps the c2 engine took including state machine decisions, inputs and outputs at each step as well as any artifacts used
- ticket: a ticket is a piece of work to be completed. a ticket may be created by a human or machine actor. tickets are always constrained to a single cell. Tickets automatically trigger an execution of a recipe. This is known as the primary job or workflow. The specific recipe triggered is either the value set on the cell or if no value set, the new ticket value set on the project

## Cells: Purpose and Parameters

Cells are a fundamental concept in c2.  Each artifact/line of code/etc of a project must belong to one and only one cell. As a project grows in complexity, the cell count for a project increases. Over time cells are added, cells subdivide, cells merge and cells are removed. Cell size is targeted to <10000 lines of code including production code, scripts, documentation, tests, etc. Each cell must have a clear mandate. Cells are responsible for ensuring they maintain their mandate by triaging incoming tasks against that mandate. A cells mandate can change but needs to go through an evaluation proces to do so. Cells serve several critical purposes:

1. Keep context size manageable to maximize effectiveness of work being done by agents
2. Create a natural checkpoint for broad-reaching changes, ensuring that architectural and design standards are maintained.
3. Keep changeset size and compatibility rules standard.
4. Provide a sandbox boundary to keep operations "in their lane", maintaining things like encapsulation.

Commits to any repository are constrained to a single cell at a time. This is by design, allowing many tasks to be run in parallel while avoiding rebase thrashing of large changesets.

## C2 workflows and state

## Op Execution Context
Operations are executed in a working directory that contains three subdirectories:
- inbox: a location where any input artifacts are made available for the operation
- outbox: a location where an operation can output files that will be included as part of the outcome of that operation
- git: holds the cell/op specific git repository (see below for details)

Ops are generally run in a sandbox environment where access to outside resources are limited.

### Git Persistence
Each cell has an associated git repository and relative path. A recipe running in that cell will get a local copy of the associated repository (similar to git worktree). After each recipe op, C2 automatically creates a local commit of all changes to the cell's relative path. These changes are made available as thin packs as well as git diffs. (Thinpacks are effectively git changesets with a connected hash.) This git state is automatically propagated to later ops.

A job is typically run in a given ref (e.g. the main branch). Until an op mutates the underlying codebase, the ref typically moves forward to stay up to date with the latest version of the underlying ref. Once an op makes changes to the git repository, a base commit is locked and the future commits are based on top of that. This git management is automatic and independent of the underlying repo for the project.

Once a workflow is satisfied with a commit (through human and/or machine decisisions), ops can be used to rebase, squash merge, etc a commit back to the base repository. There are a number of template context variables available related to git commits (both global as well as op specific).

### Artifacts

In addition to git state, each op can work with artifacts. Artifacts are arbitrary additional files that are available in the system. Some ops declare specific input or output artifacts. Other ops use the built in inbox/outbox pattern. When an op recives a general artifact input, those files are automatically made available in the inbox directory for that op. When an op writes a file or files to an outbox, those files are automatically persisted as part of the output of that op.

## C2 CLI
Your environment is configured with the c2 cli to work with recipes. The url and project are already configured. The c2 cli supports help flag to get help on individaul commands. c2 can be used to validate and update recipes as well as create new tickets and get the outcome and story of jobs. Published recipes are the ones that are used when referenced in new ticket configuration or the child recipe op (so be sure to update AND publish a recipe to use it). There is a c2 user guide in the guides directory as well.

Your environment is already configured to point to the colony2 server and correct project so you can use c2 commands.

you should start any work by reviewing the existing recipes. You can view them by running `c2 recipe list` and then `c2 recipe get <recipe-name`. 

