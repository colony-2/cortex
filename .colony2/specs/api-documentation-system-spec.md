# API Documentation System Specification

## Overview
A two-part system for generating and querying API documentation:
1. **Recipe Op**: Generates native documentation in `.colony2/api/<language>/` directories
2. **Query Tool**: MCP server or LLM tool that chunks and searches documentation on-demand

## Part 1: Documentation Generation Recipe Op

### Directory Structure
```
cell/
├── .colony2/
│   └── api/
│       ├── go/
│       │   ├── api.txt          # go doc -all output
│       │   └── packages.json    # go list -json output
│       ├── python/
│       │   ├── api.txt          # pydoc text output
│       │   └── api.html         # pydoc HTML output  
│       ├── javascript/
│       │   ├── api.json         # documentation.js or jsdoc output
│       │   └── api.md           # markdown output
│       └── java/
│           └── api.txt          # javadoc or other output
```

### Recipe Op Implementation (Simplified)

```go
// server/ops/pkg/apidoc/activity.go
package apidoc

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

type APIDocConfig struct {
    TokeiPath        string        `json:"tokei_path,omitempty"`
    OutputBaseDir    string        `json:"output_base_dir,omitempty"`  // Default: .colony2/api
    MinLanguageLines int           `json:"min_language_lines,omitempty"` // Min lines to generate docs
    Timeout          time.Duration `json:"timeout,omitempty"`
}

type APIDocInput struct {
    SourceDir       string   `json:"source_dir" validate:"required,dir"`
    Languages       []string `json:"languages,omitempty"`        // Optional: specific languages
    ForceRegenerate bool     `json:"force_regenerate,omitempty"` // Skip freshness check
}

type APIDocOutput struct {
    GeneratedDocs map[string][]string `json:"generated_docs"` // language -> file paths
    Errors        []string            `json:"errors,omitempty"`
}

func (a *APIDocActivity) Execute(ctx context.Context, config APIDocConfig, input APIDocInput) (APIDocOutput, error) {
    baseDir := filepath.Join(input.SourceDir, ".colony2", "api")
    
    // Step 1: Detect languages using tokei
    languages, err := a.detectLanguages(ctx, input.SourceDir)
    if err != nil {
        return APIDocOutput{}, err
    }
    
    // Step 2: Generate docs for each language
    output := APIDocOutput{
        GeneratedDocs: make(map[string][]string),
    }
    
    for lang, stats := range languages {
        // Skip small languages unless specifically requested
        if stats.Lines < config.MinLanguageLines && !contains(input.Languages, lang) {
            continue
        }
        
        langDir := filepath.Join(baseDir, normalizeLanguageName(lang))
        
        // Check if docs already exist and are fresh (just compare any doc file to newest source)
        if !input.ForceRegenerate && a.docsAreFresh(langDir, input.SourceDir) {
            // Just list existing files
            files, _ := filepath.Glob(filepath.Join(langDir, "*"))
            output.GeneratedDocs[lang] = files
            continue
        }
        
        // Generate documentation
        os.MkdirAll(langDir, 0755)
        files := a.generateDocsForLanguage(ctx, lang, input.SourceDir, langDir)
        output.GeneratedDocs[lang] = files
    }
    
    return output, nil
}

func (a *APIDocActivity) docsAreFresh(docDir, sourceDir string) bool {
    // Get newest doc file
    docFiles, err := filepath.Glob(filepath.Join(docDir, "*"))
    if err != nil || len(docFiles) == 0 {
        return false
    }
    
    var newestDocTime time.Time
    for _, f := range docFiles {
        if info, err := os.Stat(f); err == nil {
            if info.ModTime().After(newestDocTime) {
                newestDocTime = info.ModTime()
            }
        }
    }
    
    // Check if any source file is newer
    fresh := true
    filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() || strings.Contains(path, ".colony2") {
            return nil
        }
        // Only check relevant source files
        ext := filepath.Ext(path)
        if isSourceFile(ext) && info.ModTime().After(newestDocTime) {
            fresh = false
            return filepath.SkipAll
        }
        return nil
    })
    
    return fresh
}
```

### Language-Specific Generators

```bash
# go_generator.sh
#!/bin/bash
OUTPUT_DIR=$1
cd $2  # source dir

# Generate comprehensive Go documentation
go doc -all ./... > "$OUTPUT_DIR/api.txt" 2>/dev/null || true
go list -json ./... > "$OUTPUT_DIR/packages.json" 2>/dev/null || true

# Also generate for specific packages if found
for pkg in $(go list ./...); do
    pkg_name=$(basename $pkg)
    go doc -all $pkg > "$OUTPUT_DIR/${pkg_name}.txt" 2>/dev/null || true
done

echo '{"tool": "go doc", "generated_at": "'$(date -Iseconds)'"}'
```

## Part 2: Query Tool (MCP Server)

### MCP Server Implementation

```python
# server/mcp/api_search_server.py
import asyncio
import json
import os
import re
from pathlib import Path
from typing import List, Dict, Optional
from dataclasses import dataclass, asdict
import hashlib
import pickle
from datetime import datetime

import mcp.server as mcp
from mcp.server import Server
from mcp.types import Tool, TextContent

@dataclass
class Chunk:
    content: str
    name: Optional[str] = None
    type: Optional[str] = None
    language: str = None
    file_path: str = None
    line_start: int = 0
    line_end: int = 0
    
class SmartChunker:
    """Language-aware document chunker with caching"""
    
    PATTERNS = {
        'go': {
            'splits': [r'^func ', r'^type ', r'^var ', r'^const '],
            'name_extract': r'^(?:func|type|var|const)\s+(?:\([^)]+\)\s+)?(\w+)',
        },
        'python': {
            'splits': [r'^def ', r'^class ', r'^@\w+\s*\n(?:def|class) '],
            'name_extract': r'^(?:def|class)\s+(\w+)',
        },
        'javascript': {
            'splits': [r'^(?:export\s+)?(?:async\s+)?function\s+', r'^(?:export\s+)?class\s+'],
            'name_extract': r'(?:function|class|const)\s+(\w+)',
        },
    }
    
    def __init__(self, cache_dir: Path):
        self.cache_dir = cache_dir
        self.cache_dir.mkdir(parents=True, exist_ok=True)
    
    def chunk_file(self, file_path: Path, language: str) -> List[Chunk]:
        """Chunk a file with caching"""
        # Check cache
        cache_key = self._get_cache_key(file_path)
        cache_file = self.cache_dir / f"{cache_key}.pickle"
        
        # Check if cache is valid
        if cache_file.exists() and cache_file.stat().st_mtime > file_path.stat().st_mtime:
            with open(cache_file, 'rb') as f:
                return pickle.load(f)
        
        # Generate chunks
        content = file_path.read_text()
        chunks = self._chunk_content(content, language)
        
        # Add metadata
        for chunk in chunks:
            chunk.language = language
            chunk.file_path = str(file_path)
        
        # Save to cache
        with open(cache_file, 'wb') as f:
            pickle.dump(chunks, f)
        
        return chunks
    
    def _get_cache_key(self, file_path: Path) -> str:
        """Generate cache key for file"""
        full_path = str(file_path.absolute())
        return hashlib.md5(full_path.encode()).hexdigest()
    
    def _chunk_content(self, content: str, language: str) -> List[Chunk]:
        """Smart chunking based on language patterns"""
        patterns = self.PATTERNS.get(language, {})
        if not patterns:
            return self._generic_chunk(content)
        
        lines = content.split('\n')
        split_points = []
        
        # Find split points
        for i, line in enumerate(lines):
            for pattern in patterns.get('splits', []):
                if re.match(pattern, line):
                    split_points.append(i)
                    break
        
        # Create chunks
        split_points = [0] + split_points + [len(lines)]
        chunks = []
        
        for i in range(len(split_points) - 1):
            start = split_points[i]
            end = split_points[i + 1]
            
            chunk_lines = lines[start:end]
            chunk_content = '\n'.join(chunk_lines).strip()
            
            if not chunk_content:
                continue
            
            # Extract name
            name = None
            if patterns.get('name_extract'):
                match = re.match(patterns['name_extract'], chunk_lines[0])
                if match:
                    name = match.group(1)
            
            chunks.append(Chunk(
                content=chunk_content,
                name=name,
                type=self._determine_type(chunk_lines[0]),
                line_start=start + 1,
                line_end=end
            ))
        
        return chunks
    
    def _generic_chunk(self, content: str) -> List[Chunk]:
        """Fallback chunking"""
        chunks = []
        parts = re.split(r'\n{2,}', content)
        
        for i, part in enumerate(parts):
            if part.strip():
                chunks.append(Chunk(content=part.strip()))
        
        return chunks
    
    def _determine_type(self, first_line: str) -> str:
        if re.match(r'^(func|def|function)', first_line):
            return 'function'
        elif re.match(r'^(class|struct)', first_line):
            return 'class'
        elif re.match(r'^(type|interface|enum)', first_line):
            return 'type'
        elif re.match(r'^(const|var)', first_line):
            return 'constant'
        return 'unknown'

class APISearchServer:
    """MCP server for searching API documentation"""
    
    def __init__(self, api_dirs: List[Path]):
        self.api_dirs = api_dirs
        self.cache_dir = Path.home() / '.cache' / 'colony2_api_search'
        self.chunker = SmartChunker(self.cache_dir / 'chunks')
        self.chunks_by_dir = {}
        self._load_all_chunks()
    
    def _load_all_chunks(self):
        """Load and chunk all documentation files"""
        for api_dir in self.api_dirs:
            if not api_dir.exists():
                continue
            
            manifest_path = api_dir / 'manifest.json'
            if manifest_path.exists():
                manifest = json.loads(manifest_path.read_text())
                
                for lang_info in manifest['languages']:
                    lang = lang_info['language']
                    lang_dir = api_dir / lang
                    
                    if not lang_dir.exists():
                        continue
                    
                    # Chunk all doc files in language directory
                    chunks = []
                    for doc_file in lang_dir.glob('*.txt'):
                        chunks.extend(self.chunker.chunk_file(doc_file, lang))
                    for doc_file in lang_dir.glob('*.md'):
                        chunks.extend(self.chunker.chunk_file(doc_file, lang))
                    
                    self.chunks_by_dir[str(api_dir)] = chunks
    
    def search(self, query: str, max_results: int = 5, language: str = None) -> List[Dict]:
        """Search for API documentation"""
        results = []
        query_lower = query.lower()
        
        # Search all chunks
        for dir_path, chunks in self.chunks_by_dir.items():
            for chunk in chunks:
                # Filter by language if specified
                if language and chunk.language != language:
                    continue
                
                # Score based on match quality
                score = 0
                
                # Exact name match
                if chunk.name and chunk.name.lower() == query_lower:
                    score = 100
                # Name starts with query
                elif chunk.name and chunk.name.lower().startswith(query_lower):
                    score = 80
                # Name contains query
                elif chunk.name and query_lower in chunk.name.lower():
                    score = 60
                # Content contains query (whole word)
                elif re.search(r'\b' + re.escape(query_lower) + r'\b', chunk.content.lower()):
                    score = 40
                # Content contains query (substring)
                elif query_lower in chunk.content.lower():
                    score = 20
                
                if score > 0:
                    results.append({
                        'score': score,
                        'content': chunk.content,
                        'name': chunk.name,
                        'type': chunk.type,
                        'language': chunk.language,
                        'file': chunk.file_path,
                        'lines': f"{chunk.line_start}-{chunk.line_end}"
                    })
        
        # Sort and limit results
        results.sort(key=lambda x: x['score'], reverse=True)
        return results[:max_results]
    
    def get_summary(self) -> Dict:
        """Get summary of loaded documentation"""
        summary = {
            'directories': list(self.chunks_by_dir.keys()),
            'total_chunks': sum(len(chunks) for chunks in self.chunks_by_dir.values()),
            'languages': {}
        }
        
        for chunks in self.chunks_by_dir.values():
            for chunk in chunks:
                if chunk.language:
                    summary['languages'][chunk.language] = summary['languages'].get(chunk.language, 0) + 1
        
        return summary

# MCP Server setup
app = Server("api-search")
api_server = None

@app.list_tools()
async def list_tools() -> list[Tool]:
    return [
        Tool(
            name="search_api",
            description="Search API documentation for functions, types, or concepts",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query (e.g., function name)"},
                    "max_results": {"type": "integer", "default": 5},
                    "language": {"type": "string", "description": "Optional: filter by language"}
                },
                "required": ["query"]
            }
        ),
        Tool(
            name="get_api_summary",
            description="Get summary of available API documentation",
            inputSchema={"type": "object", "properties": {}}
        )
    ]

@app.call_tool()
async def call_tool(name: str, arguments: dict) -> list[TextContent]:
    global api_server
    
    if api_server is None:
        # Initialize with API directories from environment or config
        api_dirs = [
            Path(d) for d in os.environ.get('API_DOC_DIRS', '').split(':')
            if d and Path(d).exists()
        ]
        api_server = APISearchServer(api_dirs)
    
    if name == "search_api":
        results = api_server.search(
            arguments['query'],
            arguments.get('max_results', 5),
            arguments.get('language')
        )
        
        if not results:
            return [TextContent(text="No results found")]
        
        # Format results
        formatted = []
        for r in results:
            formatted.append(f"[{r['language']}] {r['name'] or 'unnamed'} ({r['type']})\n{r['content'][:500]}")
        
        return [TextContent(text="\n---\n".join(formatted))]
    
    elif name == "get_api_summary":
        summary = api_server.get_summary()
        return [TextContent(text=json.dumps(summary, indent=2))]
    
    return [TextContent(text="Unknown tool")]

async def main():
    # Run MCP server
    from mcp.server.stdio import stdio_server
    async with stdio_server() as (read_stream, write_stream):
        await app.run(read_stream, write_stream)

if __name__ == "__main__":
    asyncio.run(main())
```

### Starting the MCP Server

```bash
#!/bin/bash
# start_api_server.sh

# Find all .colony2/api directories in current project
API_DIRS=$(find . -type d -path "*/.colony2/api" | tr '\n' ':')

# Start MCP server with those directories
API_DOC_DIRS="$API_DIRS" python server/mcp/api_search_server.py
```

### LLM Integration

```yaml
# Claude MCP configuration
{
  "mcpServers": {
    "api-search": {
      "command": "python",
      "args": ["server/mcp/api_search_server.py"],
      "env": {
        "API_DOC_DIRS": ".colony2/api:../other_project/.colony2/api"
      }
    }
  }
}
```

## Caching Strategy

### What Gets Cached

1. **Documentation Files** (in `.colony2/api/`)
   - Native tool output stored directly
   - Freshness check: compare doc file mtime to source files
   - Regenerate only if source is newer

2. **Chunked Documents** (in `~/.cache/colony2_api_search/chunks/`)
   - Smart chunks of each doc file
   - Key: hash of doc file path
   - Invalidate if doc file mtime > cache mtime

3. **Search Index** (in `~/.cache/colony2_api_search/index/`)
   - Inverted index: terms → chunk IDs
   - Chunk ID → chunk content mapping
   - Rebuild if any doc file is newer than index

### Index Structure

```python
class SearchIndex:
    def __init__(self, cache_dir: Path):
        self.cache_dir = cache_dir
        self.index_file = cache_dir / "index.pickle"
        self.chunks_file = cache_dir / "chunks.pickle"
        self.index = {}  # term -> set of chunk_ids
        self.chunks = {}  # chunk_id -> chunk object
        self.file_mtimes = {}  # track source file times
    
    def needs_rebuild(self, doc_dirs: List[Path]) -> bool:
        """Check if any doc file is newer than index"""
        if not self.index_file.exists():
            return True
        
        index_mtime = self.index_file.stat().st_mtime
        
        for doc_dir in doc_dirs:
            for doc_file in doc_dir.rglob("*"):
                if doc_file.is_file():
                    if doc_file.stat().st_mtime > index_mtime:
                        return True
        return False
    
    def build_index(self, doc_dirs: List[Path]):
        """Build inverted index from all doc files"""
        self.index.clear()
        self.chunks.clear()
        chunk_id = 0
        
        for doc_dir in doc_dirs:
            # Detect language from directory name
            language = doc_dir.name
            
            for doc_file in doc_dir.glob("*"):
                if not doc_file.is_file():
                    continue
                
                # Chunk the file
                chunker = SmartChunker()
                file_chunks = chunker.chunk_file(doc_file, language)
                
                for chunk in file_chunks:
                    # Store chunk
                    self.chunks[chunk_id] = chunk
                    
                    # Index all terms in chunk
                    terms = self.extract_terms(chunk)
                    for term in terms:
                        if term not in self.index:
                            self.index[term] = set()
                        self.index[term].add(chunk_id)
                    
                    chunk_id += 1
        
        # Save to disk
        with open(self.index_file, 'wb') as f:
            pickle.dump((self.index, self.chunks), f)
    
    def extract_terms(self, chunk: Chunk) -> Set[str]:
        """Extract searchable terms from chunk"""
        terms = set()
        
        # Add the name if present
        if chunk.name:
            terms.add(chunk.name.lower())
            # Also add camelCase/snake_case parts
            terms.update(self.split_identifier(chunk.name))
        
        # Extract identifiers from content
        import re
        identifiers = re.findall(r'\b[a-zA-Z_]\w+\b', chunk.content)
        for ident in identifiers[:20]:  # Limit to avoid explosion
            terms.add(ident.lower())
        
        return terms
    
    def search(self, query: str) -> List[Chunk]:
        """Search using index"""
        if self.needs_rebuild():
            self.build_index()
        
        query_lower = query.lower()
        
        # Find matching chunk IDs
        matching_ids = set()
        
        # Exact match
        if query_lower in self.index:
            matching_ids.update(self.index[query_lower])
        
        # Prefix match
        for term in self.index:
            if term.startswith(query_lower):
                matching_ids.update(self.index[term])
        
        # Return chunks
        return [self.chunks[cid] for cid in matching_ids]

## Benefits

1. **Native Tool Output**: Each language uses its best documentation tool
2. **Smart Chunking**: Context-aware chunking for better search results
3. **Efficient Caching**: Three-level caching minimizes redundant work
4. **Multi-Project Support**: Can search across multiple projects
5. **Language Filtering**: Can search within specific languages
6. **Zero Preprocessing for Queries**: Chunks on first search, caches thereafter
7. **Portable**: Documentation in `.colony2/api` travels with the code

## Usage

```python
# In LLM conversation
"Search for the SaveUser function"
# Tool call: search_api(query="SaveUser")

"Show me all Python classes related to authentication"
# Tool call: search_api(query="auth", language="python")

"What API documentation is available?"
# Tool call: get_api_summary()
```