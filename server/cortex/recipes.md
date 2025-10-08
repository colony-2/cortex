

## Running a Ticket

A ticket is created using a new rest API that takes in the cell, actor, title and description as well as optional requirement document(s).

This API call will start a new ticket recipe execution (and will also be available as a recipe op).

A single ticket recipe will be fanned out into any number of child recipes.

The ticket Recipe is composed of the following recipe hierarchy:
- Ticket Recipe
	- Spec Recipe
	- Help Triage
	- Test Spec Recipe
	- Impl Recipe
	- Migrate Recipe

## Recipe Details

### Ticket Recipe
Ticket recipe owns the ticket and will use other recipes to push the ticket through it's lifecycle. The ticket recipe is started will be started with a rest api endpoint that takes in the actor, title and description of the task as required input.

### Plan Recipe
The plan recipe will write one or more specifications to complete the ticket. The specification process is a human/llm loop that will iterate over the description/requirements of the ticket and propose a set of specifications that can be implemented. In simple cases, this will be a single specification. For complex cases, this process may be multiple specifications along with various external dependency needs. External dependency needs include:
- Use of new libraries
- Creation of new cells
- Pull dependency on existing cells.
- New requirements for existing cells.

The plan recipe will output a sequence of recipe/recipeset ops. To be completed. For, it might output something akin to this pseudo example:
- [ImplRecipe/depcell1/doc2.md, ImplRecipe/depcell2/doc3.md]
- ImplRecipe/thiscell/doc1.md
- ImplRecipe/thiscell/doc4.md

This plan output indicates we will have two dependent cells implement doc2.md and doc3.md in parallel. Once completed, we will have this cell implement doc1.md and then doc4.md. Each of these invocations will be a discrete gitstate.

### Impl Recipe
The impl recipe is responsible for implementing a specification. This will include three distinct phases: Test Statements, Implementation and Validation.


### Migrate Recipe
For each specification produced by the spec recipe, the ticket recipe will create an impl recipe.


The child ticket recipe will identify if there are preknown dependencies on other cells.
### Test Spec Recipe
The test spec recipe will update the test statements for the module based on the new requirements. If there is no test statements yet, the recipe will create one. a test spec file for the ticket.

This recipe is reviews existing test statements for this cell and proposes modifications given the updated spec. This includes: which test statements need to be updated, what must be added, and which are no longer relevant removed. Test statements describe the expected behaviors of the cell. The cycle should be a proposal of a new test statements followed by a llm validation of the outputs. both the test statements and validation can be done using straight llm ops. The validation should include confirmation of no backwards incompatible changes, compliance to the specification and coverage. Once llm validation cycles are done (they need to give a % grade of quality of test statements for each of coverage, compat and compliance) and then we should branch on a realtively high percent on each. At that point, users will also validate the test statement changes. as part of this we should define a standard yaml structure for test statements.