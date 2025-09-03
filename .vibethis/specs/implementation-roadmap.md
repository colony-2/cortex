# Implementation Roadmap

## Phase 1: Core Infrastructure (Week 1)

### 1.1 API Generator Op
**File**: `server/ops/pkg/apigen/activity.go`
- [ ] Implement RegisterableActivity interface
- [ ] Integrate tokei for language detection
- [ ] Create language registry mapping
- [ ] Add configuration handling
- [ ] Write unit tests

### 1.2 Script Framework
**Directory**: `server/ops/pkg/apigen/scripts/`
- [ ] Create go.sh using `go doc`
- [ ] Create python.sh using `pydoc`
- [ ] Create node.sh using `typedoc`/`jsdoc`
- [ ] Create java.sh using `javadoc`
- [ ] Create fallback.sh using `ctags`
- [ ] Test each script independently

### 1.3 Recipe Integration
**File**: `recipes/cell_documentation_generator.yaml`
- [ ] Update recipe to use new api_generator op
- [ ] Remove grep/find hacks
- [ ] Test with git_file_collector
- [ ] Verify LLM native file handling
- [ ] Add comprehensive test cases

## Phase 2: Smart Chunking (Week 2)

### 2.1 Training Infrastructure
**Files**: `server/mcp/chunking/*.py`
- [ ] Implement LLMAnnotator class
- [ ] Create FeatureExtractor with 25+ features
- [ ] Build ChunkClassifierTrainer with RandomForest
- [ ] Add training data persistence
- [ ] Create bootstrap orchestrator

### 2.2 Runtime Chunker
**File**: `server/mcp/learned_chunker.py`
- [ ] Implement LearnedChunker class
- [ ] Add model loading/caching
- [ ] Create fallback chunking
- [ ] Add chunk metadata extraction
- [ ] Write performance benchmarks

### 2.3 Training Pipeline
**File**: `server/mcp/bootstrap.py`
- [ ] Create sample collection logic
- [ ] Implement one-time LLM annotation
- [ ] Add model validation with cross-validation
- [ ] Create model export/import utilities
- [ ] Build CLI for model management

## Phase 3: Query System (Week 3)

### 3.1 MCP Server
**File**: `server/mcp/api_search_server.py`
- [ ] Implement MCP server skeleton
- [ ] Add search_api tool
- [ ] Add get_api_summary tool
- [ ] Integrate with learned chunker
- [ ] Add result ranking algorithm

### 3.2 Caching Layer
**Directory**: `server/mcp/cache/`
- [ ] Implement chunk caching with pickle
- [ ] Add cache invalidation logic
- [ ] Create index caching
- [ ] Add cache statistics
- [ ] Build cache management CLI

### 3.3 Search Optimization
**File**: `server/mcp/indexer.py`
- [ ] Integrate Tantivy for full-text search
- [ ] Build inverted index
- [ ] Add term extraction
- [ ] Implement fuzzy matching
- [ ] Create index persistence

## Phase 4: Integration & Testing (Week 4)

### 4.1 End-to-End Testing
- [ ] Create test repositories for each language
- [ ] Generate documentation for all test repos
- [ ] Train chunkers on test data
- [ ] Verify search accuracy
- [ ] Benchmark performance

### 4.2 Documentation
- [ ] Write user guide
- [ ] Create API reference
- [ ] Add configuration examples
- [ ] Build troubleshooting guide
- [ ] Create video tutorials

### 4.3 Deployment
- [ ] Create Docker container
- [ ] Add CI/CD workflows
- [ ] Build installation scripts
- [ ] Create health checks
- [ ] Add monitoring/logging

## Phase 5: Optimization (Week 5)

### 5.1 Performance Tuning
- [ ] Profile generation pipeline
- [ ] Optimize chunking algorithms
- [ ] Improve search speed
- [ ] Reduce memory usage
- [ ] Add parallel processing

### 5.2 Model Improvements
- [ ] Collect more training data
- [ ] Fine-tune feature extraction
- [ ] Experiment with different classifiers
- [ ] Add ensemble methods
- [ ] Create language-specific optimizations

### 5.3 User Experience
- [ ] Add progress indicators
- [ ] Improve error messages
- [ ] Create interactive mode
- [ ] Add batch processing
- [ ] Build web UI (stretch goal)

## Deliverables by Phase

### Phase 1 Deliverables
- Working api_generator op
- All language scripts functional
- Updated recipe passing tests

### Phase 2 Deliverables
- Trained models for Go, Python, JavaScript
- Chunking accuracy >90% F1 score
- Model management CLI

### Phase 3 Deliverables
- MCP server responding to queries
- Search results <50ms latency
- Cache hit rate >80%

### Phase 4 Deliverables
- Full test coverage >80%
- Complete documentation
- Docker image published

### Phase 5 Deliverables
- 2x performance improvement
- Model accuracy >95% F1
- Optional web interface

## Risk Mitigation

### Technical Risks
1. **Tokei availability**: Fallback to simple language detection
2. **LLM costs**: Cache annotations aggressively
3. **Tantivy complexity**: Start with simple regex search
4. **Model accuracy**: Use rule-based fallback

### Schedule Risks
1. **Dependencies**: Parallelize independent work
2. **Testing time**: Automate all tests early
3. **Integration issues**: Daily integration tests
4. **Performance**: Profile early and often

## Success Metrics

### Functional Metrics
- [ ] All major languages supported (Go, Python, JS, Java)
- [ ] Documentation generated in <10 seconds
- [ ] Search results in <100ms
- [ ] Chunking accuracy >90%

### Quality Metrics
- [ ] Test coverage >80%
- [ ] Zero critical bugs
- [ ] Documentation completeness 100%
- [ ] User satisfaction >4/5

### Performance Metrics
- [ ] Generation: <10s for 10k LOC
- [ ] Indexing: <1s for 1000 chunks  
- [ ] Search: <50ms cached, <500ms cold
- [ ] Memory: <500MB for typical project

## Team Allocation

### Core Development (2 engineers)
- API generator implementation
- Script framework
- Recipe integration

### ML Engineering (1 engineer)
- Chunking system
- Model training
- Feature engineering

### Infrastructure (1 engineer)
- MCP server
- Caching layer
- Search optimization

### QA & Docs (1 engineer)
- Test automation
- Documentation
- User guides

## Timeline Summary

| Week | Focus | Deliverable |
|------|-------|-------------|
| 1 | Core Infrastructure | Working api_generator |
| 2 | Smart Chunking | Trained ML models |
| 3 | Query System | MCP server live |
| 4 | Integration | Full system tested |
| 5 | Optimization | Production ready |

## Next Steps

1. **Immediate** (Today):
   - Set up development environment
   - Create project structure
   - Begin api_generator implementation

2. **Tomorrow**:
   - Write first language script (Go)
   - Set up test repositories
   - Create CI pipeline

3. **This Week**:
   - Complete Phase 1
   - Start chunking research
   - Begin documentation

## Dependencies

### External Tools
- tokei (language detection)
- go doc, pydoc, etc. (documentation)
- Python with scikit-learn (ML)
- Tantivy Python bindings (search)

### Internal Systems
- server/ops framework
- server/git/pkg/gitcollector
- Recipe execution engine
- LLM inference with tools

## Notes

- Prioritize correctness over performance initially
- Keep chunking models small and fast
- Cache aggressively at every layer
- Make system observable with good logging
- Design for extensibility from the start