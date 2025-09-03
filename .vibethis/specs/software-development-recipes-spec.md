# Software Development Recipes Specification

## Overview
This specification defines four core recipes for software development within cells: Triage, Spec, Code, and Doc. Each recipe handles a specific phase of the development lifecycle and operates within the containerized nucleus environment.

## Recipe Architecture

### Common Recipe Components
All recipes share these base components:
- Temporal workflow definition
- Activity implementations
- LLM integration for intelligent processing
- Access to shared workspace and git packs
- Metrics and logging instrumentation

## 1. Triage Recipe

### Purpose
Handle incoming requests, decompose them into actionable tasks, create dependencies between cells, and route work appropriately.

### Workflow Definition
```yaml
apiVersion: v1
kind: Recipe
metadata:
  name: triage
  version: 1.0.0
  description: Processes incoming requests and creates actionable tasks

spec:
  type: triage
  
  workflow:
    class: TriageWorkflow
    timeout: 30m
    retries: 3
    
  activities:
    - name: ParseRequest
      description: Extract intent and requirements from request
      timeout: 5m
      
    - name: AnalyzeDependencies
      description: Identify which cells are affected
      timeout: 10m
      
    - name: DecomposeTask
      description: Break down into subtasks
      timeout: 10m
      
    - name: CreateWorkItems
      description: Create Temporal workflows for each subtask
      timeout: 5m
      
    - name: NotifyStakeholders
      description: Send notifications about new work
      timeout: 2m
  
  config:
    llm:
      model: gpt-4
      temperature: 0.3
      max_tokens: 4000
      
    decomposition:
      max_subtasks: 50
      min_confidence: 0.7
      auto_approve_threshold: 0.9
      
    routing:
      priority_levels: [critical, high, normal, low]
      default_priority: normal
      
  concurrency:
    max_parallel: 5
    singleton: false
```

### Workflow Logic
```python
class TriageWorkflow:
    def __init__(self, cell_context):
        self.context = cell_context
        self.llm = LLMAdapter(config.llm)
        
    async def execute(self, request: TriageRequest):
        # Step 1: Parse and understand the request
        parsed = await self.parse_request(request)
        
        # Step 2: Analyze cell dependencies
        dependencies = await self.analyze_dependencies(parsed)
        
        # Step 3: Decompose into subtasks
        subtasks = await self.decompose_task(parsed, dependencies)
        
        # Step 4: Create work items for each subtask
        work_items = []
        for subtask in subtasks:
            if subtask.cell_id == self.context.cell_id:
                # Internal work - route to appropriate recipe
                work_item = await self.create_internal_work(subtask)
            else:
                # External work - create cross-cell request
                work_item = await self.create_external_work(subtask)
            work_items.append(work_item)
        
        # Step 5: Notify stakeholders
        await self.notify_stakeholders(work_items)
        
        return TriageResult(
            request_id=request.id,
            work_items=work_items,
            dependencies=dependencies
        )
    
    async def parse_request(self, request):
        prompt = f"""
        Analyze this software development request:
        {request.content}
        
        Extract:
        1. Primary intent (feature, bugfix, refactor, etc.)
        2. Specific requirements
        3. Success criteria
        4. Constraints or limitations
        """
        return await self.llm.analyze(prompt)
    
    async def analyze_dependencies(self, parsed_request):
        # Query cell graph to understand dependencies
        cells = await self.context.get_cell_registry()
        
        prompt = f"""
        Given this request: {parsed_request}
        And these available cells: {cells}
        
        Identify which cells need to be involved and their dependencies.
        """
        return await self.llm.analyze(prompt)
    
    async def decompose_task(self, parsed_request, dependencies):
        prompt = f"""
        Break down this task into subtasks:
        {parsed_request}
        
        Consider these dependencies: {dependencies}
        
        Create subtasks that are:
        1. Atomic and well-defined
        2. Assignable to specific cells
        3. Properly sequenced
        """
        return await self.llm.decompose(prompt)
```

### Input/Output Schema
```typescript
interface TriageRequest {
    id: string;
    source: string;  // github_issue, slack, email, api
    content: string;
    metadata: {
        author: string;
        timestamp: Date;
        priority?: string;
        labels?: string[];
    };
}

interface TriageResult {
    request_id: string;
    work_items: WorkItem[];
    dependencies: Dependency[];
    estimated_effort?: number;
    suggested_timeline?: Timeline;
}

interface WorkItem {
    id: string;
    type: 'spec' | 'code' | 'doc' | 'external';
    cell_id: string;
    description: string;
    priority: string;
    dependencies: string[];  // Other work item IDs
}
```

## 2. Spec Recipe

### Purpose
Iterate on specifications for features, create detailed technical designs, and maintain alignment with requirements.

### Workflow Definition
```yaml
apiVersion: v1
kind: Recipe
metadata:
  name: spec
  version: 1.0.0
  description: Creates and refines technical specifications

spec:
  type: spec
  
  workflow:
    class: SpecWorkflow
    timeout: 2h
    retries: 2
    
  activities:
    - name: GatherContext
      description: Collect relevant code and docs
      timeout: 10m
      
    - name: GenerateSpec
      description: Create initial specification
      timeout: 30m
      
    - name: ValidateSpec
      description: Check spec against constraints
      timeout: 15m
      
    - name: RefineSpec
      description: Iterate based on feedback
      timeout: 30m
      
    - name: PublishSpec
      description: Save and distribute spec
      timeout: 5m
  
  config:
    llm:
      model: gpt-4
      temperature: 0.5
      max_tokens: 8000
      
    spec_format: markdown  # or yaml, json
    
    validation:
      check_completeness: true
      check_consistency: true
      check_feasibility: true
      
    iteration:
      max_rounds: 5
      convergence_threshold: 0.95
      
  concurrency:
    max_parallel: 3
    singleton: false
```

### Workflow Logic
```python
class SpecWorkflow:
    def __init__(self, cell_context):
        self.context = cell_context
        self.llm = LLMAdapter(config.llm)
        self.codebase = CodebaseAnalyzer(cell_context.codebase_path)
        
    async def execute(self, spec_request: SpecRequest):
        # Step 1: Gather context from codebase and existing specs
        context = await self.gather_context(spec_request)
        
        # Step 2: Generate initial specification
        spec = await self.generate_spec(spec_request, context)
        
        # Step 3: Iterative refinement loop
        for iteration in range(config.iteration.max_rounds):
            # Validate the spec
            validation = await self.validate_spec(spec, context)
            
            if validation.score >= config.iteration.convergence_threshold:
                break
                
            # Refine based on validation feedback
            spec = await self.refine_spec(spec, validation.feedback)
        
        # Step 4: Publish the final spec
        published = await self.publish_spec(spec)
        
        return SpecResult(
            spec_id=published.id,
            content=spec,
            validation_score=validation.score,
            iterations=iteration + 1
        )
    
    async def gather_context(self, request):
        # Analyze existing code structure
        code_context = await self.codebase.analyze_structure(
            request.target_path
        )
        
        # Find related specifications
        related_specs = await self.context.specs.find_related(
            request.keywords
        )
        
        # Get architectural constraints
        constraints = await self.context.get_architectural_constraints()
        
        return {
            'code': code_context,
            'specs': related_specs,
            'constraints': constraints
        }
    
    async def generate_spec(self, request, context):
        prompt = f"""
        Create a technical specification for:
        {request.description}
        
        Context:
        - Existing code structure: {context['code']}
        - Related specs: {context['specs']}
        - Constraints: {context['constraints']}
        
        Include:
        1. Overview and goals
        2. Technical approach
        3. API/Interface design
        4. Data structures
        5. Implementation phases
        6. Testing strategy
        7. Migration plan (if applicable)
        """
        
        return await self.llm.generate(prompt, format='markdown')
    
    async def validate_spec(self, spec, context):
        validations = []
        
        # Check completeness
        if config.validation.check_completeness:
            completeness = await self.check_completeness(spec)
            validations.append(completeness)
        
        # Check consistency with existing code
        if config.validation.check_consistency:
            consistency = await self.check_consistency(spec, context)
            validations.append(consistency)
        
        # Check technical feasibility
        if config.validation.check_feasibility:
            feasibility = await self.check_feasibility(spec)
            validations.append(feasibility)
        
        score = sum(v.score for v in validations) / len(validations)
        feedback = [v.feedback for v in validations if v.feedback]
        
        return ValidationResult(score=score, feedback=feedback)
```

### Input/Output Schema
```typescript
interface SpecRequest {
    id: string;
    work_item_id: string;
    description: string;
    target_path: string;
    keywords: string[];
    requirements: Requirement[];
}

interface SpecResult {
    spec_id: string;
    content: string;  // Markdown formatted spec
    validation_score: number;
    iterations: number;
    artifacts: {
        diagrams?: string[];
        examples?: CodeExample[];
        test_cases?: TestCase[];
    };
}
```

## 3. Code Recipe

### Purpose
Implement code changes based on specifications, ensure quality and consistency, manage git operations. Only one instance runs at a time to prevent merge conflicts.

### Workflow Definition
```yaml
apiVersion: v1
kind: Recipe
metadata:
  name: code
  version: 1.0.0
  description: Implements code changes based on specifications

spec:
  type: code
  
  workflow:
    class: CodeWorkflow
    timeout: 4h
    retries: 1
    
  activities:
    - name: PrepareWorkspace
      description: Setup git branch and workspace
      timeout: 5m
      
    - name: AnalyzeSpec
      description: Parse spec into implementation tasks
      timeout: 10m
      
    - name: GenerateCode
      description: Create/modify code files
      timeout: 2h
      
    - name: RunTests
      description: Execute test suite
      timeout: 30m
      
    - name: ApplyLinting
      description: Format and lint code
      timeout: 10m
      
    - name: CreateCommit
      description: Commit changes to git
      timeout: 5m
      
    - name: CreatePullRequest
      description: Open PR if configured
      timeout: 5m
  
  config:
    llm:
      model: gpt-4
      temperature: 0.2  # Lower temperature for code generation
      max_tokens: 8000
      
    code_generation:
      language_models:
        python: codex-python
        javascript: codex-js
        go: codex-go
      style_guide: /workspace/style-guide.md
      
    quality:
      run_tests: true
      require_tests_pass: true
      run_linting: true
      check_coverage: true
      min_coverage: 80
      
    git:
      branch_prefix: "cell-"
      commit_style: conventional  # conventional, semantic, custom
      sign_commits: false
      
  concurrency:
    max_parallel: 1
    singleton: true  # Critical: only one code recipe at a time
```

### Workflow Logic
```python
class CodeWorkflow:
    def __init__(self, cell_context):
        self.context = cell_context
        self.llm = LLMAdapter(config.llm)
        self.git = GitManager(cell_context.git_packs_path)
        self.codebase = CodebaseManager(cell_context.codebase_path)
        self.lock = None
        
    async def execute(self, code_request: CodeRequest):
        # Acquire exclusive lock for this cell's code modifications
        self.lock = await self.acquire_code_lock()
        
        try:
            # Step 1: Prepare workspace and branch
            workspace = await self.prepare_workspace(code_request)
            
            # Step 2: Analyze spec to create implementation plan
            plan = await self.analyze_spec(code_request.spec)
            
            # Step 3: Generate code for each planned change
            changes = []
            for task in plan.tasks:
                code = await self.generate_code(task, workspace)
                changes.append(code)
            
            # Step 4: Run tests to verify changes
            test_results = await self.run_tests(workspace)
            
            if not test_results.passed and config.quality.require_tests_pass:
                # Attempt to fix failing tests
                fixes = await self.fix_failing_tests(test_results, changes)
                changes.extend(fixes)
                test_results = await self.run_tests(workspace)
            
            # Step 5: Apply code formatting and linting
            if config.quality.run_linting:
                await self.apply_linting(workspace)
            
            # Step 6: Create git commit
            commit = await self.create_commit(changes, code_request)
            
            # Step 7: Optionally create pull request
            pr = None
            if code_request.create_pr:
                pr = await self.create_pull_request(commit, code_request)
            
            return CodeResult(
                commit_id=commit.id,
                changes=changes,
                test_results=test_results,
                pull_request=pr
            )
            
        finally:
            # Always release the lock
            await self.release_code_lock()
    
    async def acquire_code_lock(self):
        """Acquire exclusive lock for code modifications"""
        lock_key = f"cell:{self.context.cell_id}:code_lock"
        
        # Try to acquire lock with timeout
        for attempt in range(60):  # Wait up to 5 minutes
            if await self.context.acquire_lock(lock_key, ttl=3600):
                return lock_key
            await asyncio.sleep(5)
        
        raise Exception("Could not acquire code lock")
    
    async def generate_code(self, task, workspace):
        # Read existing file if modifying
        existing_code = ""
        if task.file_path and await workspace.file_exists(task.file_path):
            existing_code = await workspace.read_file(task.file_path)
        
        prompt = f"""
        Task: {task.description}
        Specification: {task.spec_section}
        
        {"Existing code:\n" + existing_code if existing_code else "Creating new file"}
        
        Generate code that:
        1. Follows the specification exactly
        2. Maintains consistency with existing code style
        3. Includes appropriate error handling
        4. Is well-documented
        5. Follows these patterns: {config.code_generation.style_guide}
        """
        
        generated = await self.llm.generate_code(
            prompt,
            language=task.language,
            model=config.code_generation.language_models.get(task.language)
        )
        
        # Write the generated code
        await workspace.write_file(task.file_path, generated.code)
        
        # Generate tests if needed
        if task.requires_tests:
            test_code = await self.generate_tests(generated.code, task)
            await workspace.write_file(task.test_path, test_code)
        
        return CodeChange(
            file_path=task.file_path,
            diff=generated.diff,
            test_path=task.test_path if task.requires_tests else None
        )
    
    async def run_tests(self, workspace):
        # Detect test framework
        framework = await self.detect_test_framework(workspace)
        
        # Run tests based on framework
        if framework == 'pytest':
            result = await workspace.run_command('pytest -v')
        elif framework == 'jest':
            result = await workspace.run_command('npm test')
        elif framework == 'go':
            result = await workspace.run_command('go test ./...')
        else:
            return TestResults(passed=True, skipped=True)
        
        # Parse test output
        return self.parse_test_results(result, framework)
    
    async def create_commit(self, changes, request):
        # Stage changes
        for change in changes:
            await self.git.add(change.file_path)
            if change.test_path:
                await self.git.add(change.test_path)
        
        # Generate commit message
        message = self.generate_commit_message(changes, request)
        
        # Create commit
        return await self.git.commit(message, sign=config.git.sign_commits)
```

### Input/Output Schema
```typescript
interface CodeRequest {
    id: string;
    spec_id: string;
    spec: Specification;
    target_branch?: string;
    create_pr: boolean;
    pr_reviewers?: string[];
}

interface CodeResult {
    commit_id: string;
    branch: string;
    changes: CodeChange[];
    test_results: TestResults;
    coverage?: CoverageReport;
    pull_request?: PullRequest;
}

interface CodeChange {
    file_path: string;
    diff: string;
    lines_added: number;
    lines_removed: number;
    test_path?: string;
}
```

## 4. Doc Recipe

### Purpose
Generate and maintain documentation for code changes, create user guides, update API references, and ensure documentation stays synchronized with code.

### Workflow Definition
```yaml
apiVersion: v1
kind: Recipe
metadata:
  name: doc
  version: 1.0.0
  description: Creates and updates documentation

spec:
  type: doc
  
  workflow:
    class: DocWorkflow
    timeout: 1h
    retries: 2
    
  activities:
    - name: AnalyzeChanges
      description: Understand what needs documentation
      timeout: 10m
      
    - name: GenerateDocumentation
      description: Create documentation content
      timeout: 30m
      
    - name: UpdateReferences
      description: Update API refs and cross-references
      timeout: 15m
      
    - name: GenerateExamples
      description: Create code examples
      timeout: 15m
      
    - name: ValidateLinks
      description: Check all documentation links
      timeout: 5m
      
    - name: PublishDocs
      description: Deploy documentation
      timeout: 5m
  
  config:
    llm:
      model: gpt-4
      temperature: 0.7
      max_tokens: 6000
      
    documentation:
      formats: [markdown, html, pdf]
      languages: [en]  # Could support multiple languages
      
    types:
      - api_reference
      - user_guide
      - developer_guide
      - changelog
      - readme
      
    generation:
      include_examples: true
      include_diagrams: true
      auto_toc: true
      
    publishing:
      targets:
        - type: filesystem
          path: /workspace/docs
        - type: confluence
          enabled: false
        - type: github_wiki
          enabled: false
      
  concurrency:
    max_parallel: 3
    singleton: false
```

### Workflow Logic
```python
class DocWorkflow:
    def __init__(self, cell_context):
        self.context = cell_context
        self.llm = LLMAdapter(config.llm)
        self.codebase = CodebaseAnalyzer(cell_context.codebase_path)
        self.doc_manager = DocumentationManager(cell_context.specs_path)
        
    async def execute(self, doc_request: DocRequest):
        # Step 1: Analyze what needs documentation
        analysis = await self.analyze_changes(doc_request)
        
        # Step 2: Generate documentation for each component
        docs = []
        for component in analysis.components:
            doc = await self.generate_documentation(component)
            docs.append(doc)
        
        # Step 3: Update cross-references and API docs
        references = await self.update_references(docs, analysis)
        
        # Step 4: Generate code examples
        if config.generation.include_examples:
            examples = await self.generate_examples(docs, analysis)
            for doc, example_set in zip(docs, examples):
                doc.examples = example_set
        
        # Step 5: Validate all links and references
        validation = await self.validate_links(docs)
        if validation.broken_links:
            docs = await self.fix_broken_links(docs, validation.broken_links)
        
        # Step 6: Publish documentation
        published = await self.publish_docs(docs)
        
        return DocResult(
            doc_ids=[d.id for d in published],
            formats=config.documentation.formats,
            components_documented=len(analysis.components),
            examples_generated=sum(len(d.examples) for d in docs)
        )
    
    async def analyze_changes(self, request):
        # Get code changes from recent commits or PRs
        if request.commit_id:
            changes = await self.codebase.get_commit_changes(request.commit_id)
        elif request.pr_id:
            changes = await self.codebase.get_pr_changes(request.pr_id)
        else:
            changes = await self.analyze_spec_for_docs(request.spec_id)
        
        # Identify components needing documentation
        components = []
        for change in changes:
            if self.needs_documentation(change):
                component = await self.extract_component(change)
                components.append(component)
        
        return AnalysisResult(
            components=components,
            existing_docs=await self.find_existing_docs(components),
            doc_standards=await self.get_doc_standards()
        )
    
    async def generate_documentation(self, component):
        # Determine documentation type needed
        doc_type = self.determine_doc_type(component)
        
        prompt = f"""
        Generate {doc_type} documentation for:
        Component: {component.name}
        Type: {component.type}
        Code: {component.code}
        
        Include:
        1. Overview and purpose
        2. {"API reference" if component.type == 'api' else "Usage instructions"}
        3. Parameters/Configuration
        4. Return values/Output
        5. Error handling
        6. Examples
        7. Related components
        
        Follow these standards: {self.context.doc_standards}
        """
        
        content = await self.llm.generate(prompt, format='markdown')
        
        # Generate diagrams if applicable
        diagrams = []
        if config.generation.include_diagrams and component.type in ['architecture', 'workflow']:
            diagram = await self.generate_diagram(component)
            diagrams.append(diagram)
        
        return Documentation(
            component_id=component.id,
            type=doc_type,
            content=content,
            diagrams=diagrams,
            metadata={
                'generated_at': datetime.now(),
                'llm_model': config.llm.model,
                'version': component.version
            }
        )
    
    async def generate_examples(self, docs, analysis):
        examples_list = []
        
        for doc in docs:
            component = next(c for c in analysis.components if c.id == doc.component_id)
            
            prompt = f"""
            Create practical code examples for:
            {doc.content}
            
            Generate {3 if component.complexity == 'high' else 2} examples:
            1. Basic usage
            2. {"Advanced usage" if component.complexity == 'high' else "Common use case"}
            3. {"Edge cases" if component.complexity == 'high' else ""}
            
            Make examples:
            - Runnable and complete
            - Well-commented
            - Demonstrate best practices
            - Show error handling
            """
            
            examples = await self.llm.generate_code_examples(prompt)
            examples_list.append(examples)
        
        return examples_list
    
    async def update_references(self, docs, analysis):
        # Find all existing references to updated components
        references = await self.doc_manager.find_references(
            [c.id for c in analysis.components]
        )
        
        updates = []
        for ref in references:
            # Update the reference with new information
            updated_ref = await self.update_single_reference(ref, docs)
            updates.append(updated_ref)
        
        # Update API reference index
        if any(d.type == 'api_reference' for d in docs):
            await self.update_api_index(docs)
        
        # Update table of contents
        if config.generation.auto_toc:
            await self.update_toc(docs)
        
        return updates
    
    async def publish_docs(self, docs):
        published = []
        
        for target in config.publishing.targets:
            if target.get('enabled', True):
                if target['type'] == 'filesystem':
                    result = await self.publish_to_filesystem(docs, target['path'])
                elif target['type'] == 'confluence':
                    result = await self.publish_to_confluence(docs, target)
                elif target['type'] == 'github_wiki':
                    result = await self.publish_to_github_wiki(docs, target)
                
                published.append(result)
        
        return published
```

### Input/Output Schema
```typescript
interface DocRequest {
    id: string;
    trigger: 'commit' | 'pr' | 'spec' | 'manual';
    commit_id?: string;
    pr_id?: string;
    spec_id?: string;
    components?: string[];  // Specific components to document
    doc_types?: DocType[];  // Types of docs to generate
}

interface DocResult {
    doc_ids: string[];
    formats: string[];
    components_documented: number;
    examples_generated: number;
    publish_locations: PublishLocation[];
}

interface Documentation {
    id: string;
    component_id: string;
    type: DocType;
    content: string;
    examples: CodeExample[];
    diagrams: Diagram[];
    metadata: DocMetadata;
}
```

## 5. Recipe Interaction Patterns

### Sequential Processing
```
Triage → Spec → Code → Doc
```
Each recipe builds on the output of the previous one.

### Parallel Processing
Multiple Spec and Doc recipes can run simultaneously for different components.

### Feedback Loops
```
Spec ←→ Code (validation feedback)
Code ←→ Doc (sync updates)
```

### Cross-Cell Communication
```python
# Triage recipe in Cell A creates work for Cell B
async def create_cross_cell_work(self, subtask):
    return await self.temporal_client.start_workflow(
        "TriageWorkflow",
        subtask,
        id=f"cell-{subtask.target_cell_id}-{subtask.id}",
        task_queue=f"cell-{subtask.target_cell_id}-triage",
        search_attributes={
            "CellId": subtask.target_cell_id,
            "RecipeType": "triage",
            "Status": "pending",
            "RequiresNucleus": True,
            "ParentCellId": self.context.cell_id
        }
    )
```

## 6. Error Handling and Recovery

### Recipe-Level Error Handling
```python
class RecipeErrorHandler:
    async def handle_error(self, recipe_type, error, context):
        if isinstance(error, LLMError):
            # Retry with different model or parameters
            return await self.retry_with_fallback_llm(context)
        
        elif isinstance(error, GitConflictError):
            # For code recipe - attempt automatic resolution
            if recipe_type == 'code':
                return await self.resolve_git_conflict(error, context)
        
        elif isinstance(error, TestFailureError):
            # Attempt to fix tests or mark as known issue
            return await self.handle_test_failure(error, context)
        
        elif isinstance(error, DependencyError):
            # Wait for dependent work to complete
            return await self.wait_for_dependency(error.dependency_id)
        
        else:
            # Log and escalate
            await self.escalate_error(recipe_type, error, context)
```

### Compensation and Rollback
```python
class CodeRecipeCompensation:
    async def compensate(self, failed_result):
        # Rollback git changes
        await self.git.reset_hard(failed_result.previous_commit)
        
        # Clean up any partial artifacts
        await self.cleanup_workspace(failed_result.workspace)
        
        # Notify dependent workflows
        await self.notify_dependents(failed_result.id, 'failed')
```

## 7. Monitoring and Metrics

### Recipe-Specific Metrics
```yaml
metrics:
  triage:
    - requests_processed
    - average_decomposition_time
    - subtasks_created_per_request
    - cross_cell_requests_created
    
  spec:
    - specs_generated
    - iteration_count_distribution
    - validation_scores
    - spec_refinement_time
    
  code:
    - commits_created
    - lines_of_code_changed
    - test_pass_rate
    - code_generation_time
    - lock_wait_time
    
  doc:
    - docs_generated
    - examples_created
    - broken_links_fixed
    - documentation_coverage
```

### Recipe Health Indicators
```python
class RecipeHealthCheck:
    async def check_recipe_health(self, recipe_type):
        health = {
            'status': 'healthy',
            'checks': {}
        }
        
        # Check LLM connectivity
        health['checks']['llm'] = await self.check_llm_available()
        
        # Check workspace availability
        health['checks']['workspace'] = await self.check_workspace_access()
        
        # Recipe-specific checks
        if recipe_type == 'code':
            health['checks']['git'] = await self.check_git_access()
            health['checks']['lock'] = await self.check_lock_availability()
        
        elif recipe_type == 'doc':
            health['checks']['publish_targets'] = await self.check_publish_targets()
        
        # Overall status
        if any(not check for check in health['checks'].values()):
            health['status'] = 'degraded'
        
        return health
```

## 8. Configuration Management

### Global Recipe Configuration
```yaml
# /recipes/global/config.yaml
global:
  llm:
    provider: openai
    api_key: ${LLM_API_KEY}
    timeout: 60s
    retry_count: 3
    
  git:
    author_name: "Cell Bot"
    author_email: "bot@cell.system"
    
  quality:
    enforce_tests: true
    enforce_linting: true
    min_coverage: 80
```

### Cell-Specific Overrides
```yaml
# /cells/frontend/.vibethis/recipes/config.yaml
overrides:
  code:
    quality:
      min_coverage: 90  # Higher standard for frontend
      
    additional_linters:
      - eslint
      - prettier
      
  doc:
    include_storybook: true
    generate_component_docs: true
```

### Dynamic Configuration Updates
```python
class RecipeConfigManager:
    async def reload_config(self, recipe_type):
        # Load base config
        config = await self.load_global_config(recipe_type)
        
        # Apply cell overrides
        if await self.has_cell_override(recipe_type):
            override = await self.load_cell_override(recipe_type)
            config = self.merge_configs(config, override)
        
        # Validate configuration
        await self.validate_config(config, recipe_type)
        
        # Apply to running recipe
        await self.apply_config(config, recipe_type)
        
        return config
```