# Development Program

We are going to build a set of recipes that work together to manage a agent-driven development process. The recipes will
be designed to use our existing recipe and op system.

Important rules:

- A ticket can only change one cell.
- A cell can only request changes of cells it depends on.
- Only one ticket for a cell can be active at a time. (If a ticket recipe for a cell is currently active, any other ticket recipe submissions will block until the first ticket reicpe is completed.)
- Any ticket can be rewound to an earlier state and restarted.

## New Rest APIs

Introduce a new Rest API that supports two operations:

- Create Ticket
- Rewind a Ticket

### Create Ticket

Create will take in the target cell, actor, title and description as well as optional requirement/spec document(s).

Create will start a new TicketRecipe execution, described below. It is important to note that the canonical definition
of a ticket is the ticket workflow execution, no the ticket definition in postgres. The postgres version of the ticket
is there for each reporting and tracking for human consumption. Agent recipes should not need to read that data except
in as much as is required to get correct concurrency versions for update.

### Rewind Ticket

Rewind will take in the ticket id, the actor and the last valid event (which could be the initial ticket creation). It
will cancel any existing ticket recipe executions and then reset the ticket to the that state. Once reset, it will start
a new ticket recipe execution that will use the last valid event as the starting point. The restart point is likely to
include updated requirements/specs/prompts to drive things in a different direction.

Note: Both humans and agents can create tickets. Any ticket recipe execution (whether human or agent generated) can be
rewound.

## Key Recipes

### Ticket Recipe

Ticket recipe owns the ticket and will use other recipes to push the ticket through it's lifecycle. The ticket recipe is
responsible for updating the overall ticket state in the database as things progress.

The ticket recipe will go through the following steps:

1. Create a new ticket using the ticket.manage op.
2. Execute the PlanRecipe for the ticket and get back a RecipePlan (described below).
3. Post the RecipePlan to the ticket as a note.
4. Execute the proposed plan. This may include any combination of steps.
5. Execute a thinpackrebase for this cell.
6. Update the cells AGENTS.MD with the latest information about this cell using llm_inference2
6. Present a final summary of completed work for human approval. Use llm_inference2 with gemini flash 2.5 to generate
   the final summary.
7. When approved, do a squashrebasemerge and mark the ticket as complete. Use the final summary as the commit message.
   Also include the ticket id and post-rebase thinpack information for later evaluation of intermediate steps.
8. Generate a DeprecationPlan using codex.exec.
9. Execute the DeprecationPlan with the DeprecationRecipe (async recipe, discrete gitstate).
10. Recipe is finished.

A single ticket recipe can be fanned out into any number of child recipes.

#### RecipePlan

The RecipePlan is a sequence of recipe/recipeset ops. To be completed. A pseudo example:

- [TicketRecipe/depcell1/doc1.md, TicketRecipe/depcell2/doc2.md]
- ImplRecipe/thiscell/doc3.md
- ImplRecipe/thiscell/doc4.md

Need to define the specific standardized format/schema for RecipePlan.
This plan output indicates we will have two dependent cells implement doc1.md and doc2.md in parallel. Once both are
completed, we will have this cell implement doc3.md and then doc4.md using the ImplementationRecipe.

All operations for !thiscell will be done in discrete gitstates.

All operations for thiscell will be done in a single shared gitstate. After any !thiscell operations are complete,
thiscell will execute a thinpackrebase.

### Plan Recipe

The plan recipe will write one or more specifications to complete the ticket. The specification process is a human/llm
loop that will iterate over the description/requirements of the ticket and propose a set of specifications that can be
implemented. In simple cases, this may be a single specification. For complex cases, this process may be multiple
specifications along with various external dependency needs. External dependency needs include:

- Use of new libraries
- Creation of new cells
- New requirements for existing cells.

We will use codex.exec to execute the plan recipe. For each specification returned on each llm turn, we will use
ticket.manage op to add, update or delete specification md docs.

The plan recipe must not change any code in the cell and should only write specification documents in the .colony2/specs
directory (descriptive name, cell and ticketid should be in filename). The RecipePlan will be written to .colony2/plan
directory (and include descriptive name + ticketid).

The plan needs to go through a set of confrontational cycles to ensure that the specifications match are standards. We
will use independent codex sessions for the different validation cycles to avoid self-reinforcement. The cycles will be:

- Validate changes are backwards compatible
- Ensure changes cover the specification
- Remove extraneous features beyond what was required. (Avoid excessive creativity)

Each validation type will maintain it's validation session across cycles. Each validation needs to give a quality
score (1..5) on their validation type. The process will pause for human feedback via the input op if the score is below
a certain threshold after a moderate number of rounds.

### ImplRecipe

The impl recipe will implement a given specification. The impl recipe will be a sequence of operations that will be
executed in a single gitstate within a single cell.

It will include the three main phases:

- Test Statements
- Implementation
- Validation

The first phase of the impl recipe will be to have codex.exec propose a list of test statement adjustments given the
provided specification. Test statements are a set of test cases that describe the expected behavior of the cell. The
test statements are written in markdown and should be stored in the .colony2/specs directory.

Test statements must be written in a way that is easy to understand and easy to validate. The rules for test statements
are:

- **MUST** be written in markdown
- **MUST** be stored in the .colony2/specs directory
- **MUST** limit test statements to 30 words or less
- **MUST** include all affected filenames in the test statement when covering multiple files
- **MUST** use business/expectation language rather than implementation details
- **MUST** include both positive and negative test cases for critical functionality
- **MUST** focus on integration points and avoid testing trivial getters/setters

The second phase of the impl recipe will be to implement the specification. This will include the following steps:

- Implement the specification use codex.exec
- Run local tests in the cell within the given worktree
- Run tests of all downstream cells

To implement the specification, we will use codex.exec. This will be responsible for both implementing the underlying
code as well as updating the test cases to match the updated test statements.

codex.exec may return incomplete. There are two likely reasons for this:

- User input is needed. In this case, collect the user input using the input op and incorporate into a codex.exec
  execution that resumes the existing session.
- A dependency needs changes. In this case, the impl recipe will return a pendingDependencies list.

In the case of a dependency change, the impl recipe will go through a dependency resolution phase. This will include:

- For each dependency, start a new child TicketRecipe execution with the defined requirements.
- Once all dependencies are complete, the impl recipe will resume the codex session with feedback of changes from all
  the updated dependencies.

As part of the dependency change process, the impl recipe will review the the interdependency of changes. If they are
interdependent, the impl will run them in the required sequence. However, if they are not interdependent, the impl
recipe will run them in parallel.

Once codex.exec reports completion, we will run the validation phase. This will include:

- Run local tests in the cell within the given worktree
- Run tests of all downstream cells

The validation phase will be a set of independent codex.exec executions. The cycles will be:

- Validate changes are backwards compatible
- Ensure changes cover the specification
- Ensure tests statements are covered correctly.
- Remove excessive embellishments or creative flourishes.

The validation phase will report a quality score (1..5) on each validation type. The process will pause for human
feedback via the input op if the score is below a certain threshold after a moderate number of rounds.

For all issues, the validation phase will generate a list of issues that will be combined back into a feedback for the
implementation phase. The combined feedback will be fed back into the implementation codex session for correction and
the cycle will continue until validation phase score is sufficiently high.

### DeprecationRecipe

As part of each spec, the specs should include any deprecation/migration steps that must be taken to complete the
desired work. Since all changes must be backwards compatible, the deprecation recipe will be responsible for
implementing the deprecation steps after the ticket has been completed. Once the impl has been completed, we should
build a DeprecationPlan. This should be composed of:

- an ordered list of things that must be done.
- each list item should include: a concise llm to-be-consumed description of what changes should be made, a command that
  can be used to identify if this pattern exists in this directory of files

The data should be structured as a json document

Our framework will take a breaking change instruction and will do the following:

- for each deprecation step that must be done. identify each component that must have that change. for each component,
  have component owning agent apply that change.
- for any inflight changes that exist once all components for a deprecation step have been made, create a barrier that
  requires they will only be allowed to merge once they also do not match the deprecation step identifcation pattern
- after that item is done such that no components still have the problematic pattern, move to the next deprecation step
  and repeat
- the final deprecation step should be to eliminate any remaining deprecated code.

The TicketRecipe is responsible for generating the DeprecationPlan and then will start a DeprecationRecipe that will be
responsible for executing for each of the deprecation steps via creation of a number of child TicketRecipes, evaluating
the barriers at each step and confirming success of the deprecation steps before moving to the next. The deprecation
recipe is the only recipe that can generate work for cells that depend on thiscell and is executed independent of the
original .
