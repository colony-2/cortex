# Learned Documentation Chunking Specification

## Overview
A self-training system that uses LLM guidance to learn how to chunk documentation output for each programming language. The LLM annotates a small sample once, trains a classifier, then the classifier handles all future chunking without LLM calls.

## Architecture

```
.cache/colony2/
├── models/
│   ├── go_chunk_classifier.pkl
│   ├── python_chunk_classifier.pkl
│   ├── javascript_chunk_classifier.pkl
│   └── ...
├── training_data/
│   ├── go_training.json
│   ├── python_training.json
│   └── ...
└── configs/
    └── chunking_config.json
```

## Training Phase

### 1. Sample Collection

```python
class SampleCollector:
    """Collects representative documentation samples for training"""
    
    def collect_samples(self, api_dir: Path, language: str) -> List[str]:
        """
        Collect diverse documentation samples for a language.
        
        Returns:
            List of documentation strings, each 1-3KB in size
        """
        lang_dir = api_dir / language
        samples = []
        
        # Strategy: Get diverse samples
        doc_files = list(lang_dir.glob("*"))
        
        if len(doc_files) == 0:
            return []
        
        # Take beginning, middle, and end portions from different files
        for doc_file in doc_files[:3]:  # Max 3 files
            content = doc_file.read_text()
            
            # Get a chunk from the beginning (likely package docs)
            if len(content) > 1000:
                samples.append(content[:1000])
            
            # Get a chunk from the middle (likely function docs)
            if len(content) > 3000:
                mid_point = len(content) // 2
                samples.append(content[mid_point:mid_point+1000])
        
        return samples
```

### 2. LLM Annotation

```python
class LLMAnnotator:
    """Uses LLM to annotate chunk boundaries in documentation"""
    
    PROMPT_TEMPLATE = """
You are analyzing {language} documentation output from {tool}.
Your task is to identify logical chunk boundaries.

A chunk should be:
- A complete, self-contained unit of documentation
- Usually a single function, method, type, class, or constant
- Include its description, parameters, return values, etc.

For each line in the documentation, classify it as:
- "start": Begins a new chunk (function signature, type declaration, class definition)
- "continue": Continues the current chunk (description, parameters, body)
- "blank": Empty line or separator

Documentation sample:
```
{sample}
```

Respond with a JSON array of classifications, one per line.
Example: ["start", "continue", "continue", "blank", "start", "continue", ...]

Important patterns for {language}:
{language_hints}
"""
    
    LANGUAGE_HINTS = {
        'go': """
        - Functions start with 'func ' at the beginning of a line
        - Types start with 'type ' at the beginning of a line
        - Constants start with 'const ' at the beginning of a line
        - Variables start with 'var ' at the beginning of a line
        - Indented lines are descriptions/continuations
        """,
        
        'python': """
        - Classes typically shown as 'class ClassName(BaseClass):'
        - Functions shown as 'functionname(parameters)'
        - Module sections have UPPERCASE headers like 'CLASSES', 'FUNCTIONS'
        - Indented lines with '|' are continuations
        """,
        
        'javascript': """
        - Functions may appear as 'functionName(params)'
        - Classes shown as 'class ClassName'
        - May use markdown headers (###) for sections
        - JSDoc comments are part of the chunk they describe
        """,
        
        'java': """
        - Method signatures like 'public void methodName(Type param)'
        - Classes shown as 'public class ClassName'
        - Package declarations start with 'package '
        - Indented lines are descriptions
        """
    }
    
    def __init__(self, llm_client):
        self.llm_client = llm_client
    
    def annotate(self, sample: str, language: str, tool: str = None) -> List[str]:
        """
        Get LLM annotations for a documentation sample.
        
        Args:
            sample: Documentation text to annotate
            language: Programming language
            tool: Documentation tool used (e.g., 'go doc', 'pydoc')
            
        Returns:
            List of labels, one per line
        """
        if not tool:
            tool_map = {
                'go': 'go doc',
                'python': 'pydoc',
                'javascript': 'jsdoc/documentation.js',
                'java': 'javadoc'
            }
            tool = tool_map.get(language, 'documentation tool')
        
        prompt = self.PROMPT_TEMPLATE.format(
            language=language,
            tool=tool,
            sample=sample,
            language_hints=self.LANGUAGE_HINTS.get(language, "Review the format and identify natural boundaries.")
        )
        
        response = self.llm_client.complete(prompt)
        
        try:
            annotations = json.loads(response)
            
            # Validate length matches
            sample_lines = sample.split('\n')
            if len(annotations) != len(sample_lines):
                raise ValueError(f"Annotation count {len(annotations)} doesn't match line count {len(sample_lines)}")
            
            return annotations
            
        except (json.JSONDecodeError, ValueError) as e:
            # Fallback to simple heuristics if LLM fails
            return self.fallback_annotate(sample, language)
    
    def fallback_annotate(self, sample: str, language: str) -> List[str]:
        """Simple rule-based fallback"""
        annotations = []
        for line in sample.split('\n'):
            if not line.strip():
                annotations.append('blank')
            elif not line[0].isspace():  # Not indented
                annotations.append('start')
            else:
                annotations.append('continue')
        return annotations
```

### 3. Feature Extraction

```python
class FeatureExtractor:
    """Extracts features from documentation lines for classification"""
    
    @staticmethod
    def extract_features(line: str, context: dict) -> np.ndarray:
        """
        Extract numerical features from a line.
        
        Args:
            line: The current line
            context: Dictionary with 'prev_line', 'next_line', 'line_index', 'language'
            
        Returns:
            Feature vector as numpy array
        """
        features = []
        
        # Basic line features
        features.append(len(line))                              # 0: line length
        features.append(len(line.strip()))                      # 1: stripped length
        features.append(len(line) - len(line.lstrip()))        # 2: indentation
        features.append(1 if line.strip() == '' else 0)        # 3: is blank
        
        # Character features
        features.append(1 if line and line[0].isupper() else 0)  # 4: starts with capital
        features.append(1 if line and line[0].isspace() else 0)  # 5: starts with space
        features.append(line.count('('))                          # 6: open paren count
        features.append(line.count(')'))                          # 7: close paren count
        features.append(line.count('{'))                          # 8: open brace count
        features.append(line.count(':'))                          # 9: colon count
        
        # Word features
        words = line.split()
        features.append(len(words))                               # 10: word count
        first_word = words[0] if words else ''
        features.append(1 if first_word in ['func', 'type', 'const', 'var'] else 0)  # 11: go keywords
        features.append(1 if first_word in ['def', 'class'] else 0)                  # 12: python keywords
        features.append(1 if first_word in ['public', 'private', 'protected'] else 0) # 13: java keywords
        features.append(1 if first_word in ['function', 'class', 'export'] else 0)    # 14: js keywords
        
        # Context features
        prev_line = context.get('prev_line', '')
        next_line = context.get('next_line', '')
        
        features.append(1 if prev_line.strip() == '' else 0)     # 15: prev is blank
        features.append(1 if next_line.strip() == '' else 0)     # 16: next is blank
        features.append(len(prev_line) - len(prev_line.lstrip())) # 17: prev indentation
        features.append(len(next_line) - len(next_line.lstrip())) # 18: next indentation
        
        # Pattern features
        features.append(1 if '->' in line else 0)                 # 19: has return type arrow
        features.append(1 if 'return' in line.lower() else 0)    # 20: has return keyword
        features.append(1 if re.match(r'^[A-Z_]+$', first_word) else 0) # 21: all caps first word
        features.append(1 if line.strip().endswith(':') else 0)  # 22: ends with colon
        
        # Position features
        line_index = context.get('line_index', 0)
        features.append(line_index)                               # 23: absolute position
        features.append(1 if line_index < 5 else 0)              # 24: near start
        
        return np.array(features, dtype=np.float32)
```

### 4. Classifier Training

```python
from sklearn.ensemble import RandomForestClassifier
from sklearn.model_selection import cross_val_score
import joblib

class ChunkClassifierTrainer:
    """Trains a classifier to identify chunk boundaries"""
    
    def __init__(self, language: str):
        self.language = language
        self.model_path = Path.home() / '.cache' / 'colony2' / 'models' / f'{language}_chunk_classifier.pkl'
        self.training_data_path = Path.home() / '.cache' / 'colony2' / 'training_data' / f'{language}_training.json'
        self.extractor = FeatureExtractor()
    
    def prepare_training_data(self, samples: List[str], annotations: List[List[str]]) -> Tuple[np.ndarray, np.ndarray]:
        """
        Convert samples and annotations to feature matrix and labels.
        
        Returns:
            X: Feature matrix (n_samples, n_features)
            y: Labels array (n_samples,)
        """
        X = []
        y = []
        
        for sample, sample_annotations in zip(samples, annotations):
            lines = sample.split('\n')
            
            for i, (line, label) in enumerate(zip(lines, sample_annotations)):
                context = {
                    'prev_line': lines[i-1] if i > 0 else '',
                    'next_line': lines[i+1] if i < len(lines)-1 else '',
                    'line_index': i,
                    'language': self.language
                }
                
                features = self.extractor.extract_features(line, context)
                X.append(features)
                y.append(label)
        
        return np.array(X), np.array(y)
    
    def train(self, X: np.ndarray, y: np.ndarray) -> RandomForestClassifier:
        """
        Train the chunk classifier.
        
        Returns:
            Trained classifier
        """
        # Use RandomForest for interpretability and speed
        classifier = RandomForestClassifier(
            n_estimators=100,
            max_depth=15,
            min_samples_split=5,
            min_samples_leaf=2,
            random_state=42
        )
        
        # Train
        classifier.fit(X, y)
        
        # Evaluate with cross-validation
        scores = cross_val_score(classifier, X, y, cv=3, scoring='f1_macro')
        print(f"Cross-validation F1 score for {self.language}: {scores.mean():.3f} (+/- {scores.std():.3f})")
        
        # Save model
        self.model_path.parent.mkdir(parents=True, exist_ok=True)
        joblib.dump(classifier, self.model_path)
        
        # Save training data for inspection/retraining
        self.training_data_path.parent.mkdir(parents=True, exist_ok=True)
        training_record = {
            'language': self.language,
            'n_samples': len(y),
            'feature_names': self.get_feature_names(),
            'feature_importance': classifier.feature_importances_.tolist(),
            'classes': classifier.classes_.tolist(),
            'accuracy': scores.mean()
        }
        
        with open(self.training_data_path, 'w') as f:
            json.dump(training_record, f, indent=2)
        
        return classifier
    
    def get_feature_names(self) -> List[str]:
        """Return feature names for interpretability"""
        return [
            'line_length', 'stripped_length', 'indentation', 'is_blank',
            'starts_capital', 'starts_space', 'open_paren_count', 'close_paren_count',
            'open_brace_count', 'colon_count', 'word_count',
            'has_go_keyword', 'has_python_keyword', 'has_java_keyword', 'has_js_keyword',
            'prev_is_blank', 'next_is_blank', 'prev_indent', 'next_indent',
            'has_return_arrow', 'has_return_word', 'all_caps_first', 'ends_colon',
            'line_index', 'near_start'
        ]
```

## Inference Phase

### 5. Runtime Chunking

```python
class LearnedChunker:
    """Uses trained classifier to chunk documentation at runtime"""
    
    def __init__(self, language: str):
        self.language = language
        self.model_path = Path.home() / '.cache' / 'colony2' / 'models' / f'{language}_chunk_classifier.pkl'
        self.extractor = FeatureExtractor()
        self.classifier = None
        self._load_model()
    
    def _load_model(self):
        """Load trained model if available"""
        if self.model_path.exists():
            self.classifier = joblib.load(self.model_path)
    
    def is_trained(self) -> bool:
        """Check if a trained model is available"""
        return self.classifier is not None
    
    def chunk(self, content: str) -> List[Dict[str, Any]]:
        """
        Chunk documentation using trained classifier.
        
        Returns:
            List of chunk dictionaries with 'content', 'start_line', 'end_line'
        """
        if not self.classifier:
            return self.fallback_chunk(content)
        
        lines = content.split('\n')
        
        # Predict labels for all lines
        predictions = []
        for i, line in enumerate(lines):
            context = {
                'prev_line': lines[i-1] if i > 0 else '',
                'next_line': lines[i+1] if i < len(lines)-1 else '',
                'line_index': i,
                'language': self.language
            }
            
            features = self.extractor.extract_features(line, context)
            prediction = self.classifier.predict([features])[0]
            predictions.append(prediction)
        
        # Group into chunks based on predictions
        chunks = []
        current_chunk_lines = []
        current_chunk_start = 0
        
        for i, (line, pred) in enumerate(zip(lines, predictions)):
            if pred == 'start' and current_chunk_lines:
                # Save current chunk
                chunks.append({
                    'content': '\n'.join(current_chunk_lines),
                    'start_line': current_chunk_start,
                    'end_line': i - 1,
                    'name': self.extract_name(current_chunk_lines[0])
                })
                current_chunk_lines = [line]
                current_chunk_start = i
            else:
                current_chunk_lines.append(line)
        
        # Don't forget last chunk
        if current_chunk_lines:
            chunks.append({
                'content': '\n'.join(current_chunk_lines),
                'start_line': current_chunk_start,
                'end_line': len(lines) - 1,
                'name': self.extract_name(current_chunk_lines[0])
            })
        
        return chunks
    
    def extract_name(self, first_line: str) -> Optional[str]:
        """Try to extract a name from the first line of a chunk"""
        patterns = [
            r'func\s+(?:\([^)]+\)\s+)?(\w+)',  # Go method/function
            r'type\s+(\w+)',                     # Go type
            r'def\s+(\w+)',                      # Python function
            r'class\s+(\w+)',                    # Python/Java class
            r'(?:public|private|protected)\s+\w+\s+(\w+)\s*\(',  # Java method
            r'^(\w+)\s*\(',                      # Generic function
        ]
        
        for pattern in patterns:
            if match := re.search(pattern, first_line):
                return match.group(1)
        
        return None
    
    def fallback_chunk(self, content: str) -> List[Dict[str, Any]]:
        """Simple fallback when no trained model available"""
        chunks = []
        current_chunk = []
        
        for i, line in enumerate(content.split('\n')):
            # Simple heuristic: non-indented non-empty lines start chunks
            if line and not line[0].isspace() and current_chunk:
                chunks.append({
                    'content': '\n'.join(current_chunk),
                    'start_line': i - len(current_chunk),
                    'end_line': i - 1
                })
                current_chunk = [line]
            else:
                current_chunk.append(line)
        
        if current_chunk:
            chunks.append({
                'content': '\n'.join(current_chunk),
                'start_line': len(content.split('\n')) - len(current_chunk),
                'end_line': len(content.split('\n')) - 1
            })
        
        return chunks
```

## Training Orchestration

### 6. Bootstrap Process

```python
class ChunkingSystemBootstrap:
    """Orchestrates the training of chunk classifiers"""
    
    def __init__(self, llm_client):
        self.llm_client = llm_client
        self.config_path = Path.home() / '.cache' / 'colony2' / 'configs' / 'chunking_config.json'
    
    def bootstrap(self, api_dirs: List[Path], force_retrain: bool = False):
        """
        Bootstrap classifiers for all languages found in API directories.
        
        Args:
            api_dirs: List of .colony2/api directories
            force_retrain: Retrain even if models exist
        """
        languages_seen = set()
        
        for api_dir in api_dirs:
            if not api_dir.exists():
                continue
            
            # Find all language subdirectories
            for lang_dir in api_dir.iterdir():
                if not lang_dir.is_dir() or lang_dir.name.startswith('.'):
                    continue
                
                language = lang_dir.name
                
                if language in languages_seen and not force_retrain:
                    continue
                
                languages_seen.add(language)
                
                # Check if already trained
                model_path = Path.home() / '.cache' / 'colony2' / 'models' / f'{language}_chunk_classifier.pkl'
                if model_path.exists() and not force_retrain:
                    print(f"Model for {language} already exists, skipping...")
                    continue
                
                print(f"Training chunker for {language}...")
                self.train_for_language(api_dir, language)
    
    def train_for_language(self, api_dir: Path, language: str):
        """Train a classifier for a specific language"""
        
        # Collect samples
        collector = SampleCollector()
        samples = collector.collect_samples(api_dir, language)
        
        if not samples:
            print(f"No samples found for {language}, skipping...")
            return
        
        # Get LLM annotations
        annotator = LLMAnnotator(self.llm_client)
        all_annotations = []
        
        for sample in samples:
            annotations = annotator.annotate(sample, language)
            all_annotations.append(annotations)
        
        # Train classifier
        trainer = ChunkClassifierTrainer(language)
        X, y = trainer.prepare_training_data(samples, all_annotations)
        
        if len(X) < 50:
            print(f"Warning: Only {len(X)} training samples for {language}")
        
        classifier = trainer.train(X, y)
        print(f"Successfully trained {language} classifier")
        
        # Save config
        self.update_config(language, len(X))
    
    def update_config(self, language: str, n_samples: int):
        """Update configuration file with training info"""
        config = {}
        
        if self.config_path.exists():
            with open(self.config_path) as f:
                config = json.load(f)
        
        if 'languages' not in config:
            config['languages'] = {}
        
        config['languages'][language] = {
            'trained_at': datetime.now().isoformat(),
            'n_samples': n_samples,
            'model_version': '1.0'
        }
        
        self.config_path.parent.mkdir(parents=True, exist_ok=True)
        with open(self.config_path, 'w') as f:
            json.dump(config, f, indent=2)
```

## Integration with Tantivy Index

### 7. Smart Indexing

```python
class SmartTantivyIndexer:
    """Tantivy indexer using learned chunkers"""
    
    def __init__(self, api_dir: Path):
        self.api_dir = api_dir
        self.index_dir = api_dir / '.index'
        self.chunkers = {}  # Cache of language -> LearnedChunker
    
    def get_chunker(self, language: str) -> LearnedChunker:
        """Get or create chunker for language"""
        if language not in self.chunkers:
            self.chunkers[language] = LearnedChunker(language)
        return self.chunkers[language]
    
    def index_documentation(self):
        """Index all documentation using learned chunkers"""
        
        # ... Tantivy setup code ...
        
        for lang_dir in self.api_dir.iterdir():
            if not lang_dir.is_dir() or lang_dir.name.startswith('.'):
                continue
            
            language = lang_dir.name
            chunker = self.get_chunker(language)
            
            if not chunker.is_trained():
                print(f"Warning: No trained chunker for {language}, using fallback")
            
            for doc_file in lang_dir.glob("*"):
                if not doc_file.is_file():
                    continue
                
                content = doc_file.read_text()
                chunks = chunker.chunk(content)
                
                for chunk in chunks:
                    # Add to Tantivy index
                    doc = tantivy.Document()
                    doc.add_text("content", chunk['content'])
                    doc.add_text("name", chunk.get('name', ''))
                    doc.add_text("language", language)
                    doc.add_text("file", str(doc_file))
                    doc.add_unsigned("start_line", chunk['start_line'])
                    doc.add_unsigned("end_line", chunk['end_line'])
                    
                    writer.add_document(doc)
```

## Usage

```python
# One-time bootstrap (could be part of initial setup)
llm_client = YourLLMClient()
bootstrap = ChunkingSystemBootstrap(llm_client)
bootstrap.bootstrap([Path(".colony2/api")])

# Runtime usage (no LLM needed)
indexer = SmartTantivyIndexer(Path(".colony2/api"))
indexer.index_documentation()

# Chunker can be used standalone
chunker = LearnedChunker("go")
if chunker.is_trained():
    chunks = chunker.chunk(go_doc_content)
```

## Benefits

1. **One-time LLM cost**: Only need LLM during initial training
2. **Fast inference**: RandomForest prediction is microseconds per line
3. **Language-specific**: Each language gets its own trained model
4. **Self-improving**: Can retrain with more/better samples
5. **Interpretable**: Can inspect feature importance
6. **Robust**: Fallback to simple heuristics if model unavailable
7. **Portable**: Models can be shared across machines

## Model Management

```bash
# CLI commands for model management
colony2-chunker train --language go --samples /path/to/samples
colony2-chunker list    # List trained models
colony2-chunker test --language python --file test.txt
colony2-chunker export --language go --output go_model.pkl
colony2-chunker import --language go --input go_model.pkl
```