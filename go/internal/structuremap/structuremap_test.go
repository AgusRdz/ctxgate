package structuremap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 1. IsStructureSupported
// ---------------------------------------------------------------------------

func TestIsStructureSupported(t *testing.T) {
	assert.True(t, IsStructureSupported("foo.py"))
	assert.True(t, IsStructureSupported("foo.ts"))
	assert.True(t, IsStructureSupported("foo.tsx"))
	assert.True(t, IsStructureSupported("foo.js"))
	assert.True(t, IsStructureSupported("foo.md"))
	assert.True(t, IsStructureSupported("foo.json"))
	assert.True(t, IsStructureSupported("foo.yaml"))
	assert.True(t, IsStructureSupported("foo.toml"))
	assert.False(t, IsStructureSupported("foo.go"))
	assert.False(t, IsStructureSupported("foo.png"))
	assert.False(t, IsStructureSupported("foo.rs"))
	assert.False(t, IsStructureSupported("foo"))
}

// ---------------------------------------------------------------------------
// 2. DetectLanguage
// ---------------------------------------------------------------------------

func TestDetectLanguage(t *testing.T) {
	assert.Equal(t, "python", DetectLanguage("foo.py"))
	assert.Equal(t, "typescript", DetectLanguage("foo.ts"))
	assert.Equal(t, "typescript", DetectLanguage("foo.tsx"))
	assert.Equal(t, "javascript", DetectLanguage("foo.js"))
	assert.Equal(t, "javascript", DetectLanguage("foo.mjs"))
	assert.Equal(t, "markdown", DetectLanguage("foo.md"))
	assert.Equal(t, "json", DetectLanguage("foo.json"))
	assert.Equal(t, "yaml", DetectLanguage("foo.yaml"))
	assert.Equal(t, "toml", DetectLanguage("foo.toml"))
	assert.Equal(t, "unknown", DetectLanguage("foo.xyz"))
	assert.Equal(t, "unknown", DetectLanguage("foo.go"))
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// bigPythonSource creates a Python source large enough to exceed minTokensForStructure=1000.
// Uses varied content to avoid triggering the generated-file heuristics.
func bigPythonSource() string {
	var sb strings.Builder
	sb.WriteString(`# Module documentation for the ctxgate service layer.
# This module provides service utilities for managing user sessions.
# It implements the core business logic for the hook processing pipeline.

import os
import sys
import json
import hashlib
from typing import List, Optional, Dict, Tuple
from pathlib import Path
from dataclasses import dataclass

@dataclass
class Config:
    """Configuration for the service."""
    name: str
    timeout: int = 30
    debug: bool = False

class MyService:
    """Primary service class providing session management and hook processing.

    This class encapsulates all business logic for interacting with the
    session store and dispatching hook events to appropriate handlers.
    """

    DEFAULT_TIMEOUT = 30
    MAX_RETRIES = 3

    def __init__(self, name: str, config: Optional[Config] = None):
        """Initialize the service with a name and optional configuration."""
        self.name = name
        self.config = config or Config(name=name)
        self._cache: Dict[str, str] = {}
        self._retries = 0

    def process(self, data: List[str]) -> bool:
        """Process a list of data items.

        Each item is validated and transformed before being committed
        to the session store. Returns True on success.
        """
        for item in data:
            if not self.validate(item):
                return False
            self._cache[item] = hashlib.sha256(item.encode()).hexdigest()
        return True

    def validate(self, item: str) -> bool:
        """Validate that an item meets minimum requirements."""
        return len(item) > 0 and not item.startswith('_')

    def reset(self) -> None:
        """Reset the internal cache and retry counter."""
        self._cache.clear()
        self._retries = 0

    def get_stats(self) -> Dict[str, int]:
        """Return current statistics about the service state."""
        return {
            'cache_size': len(self._cache),
            'retries': self._retries,
        }

class ExtendedService(MyService):
    """Extended service with additional capabilities for batch processing."""

    def __init__(self, name: str, batch_size: int = 100):
        super().__init__(name)
        self.batch_size = batch_size
        self.processed_count = 0

    def batch_process(self, items: List[str]) -> Tuple[int, int]:
        """Process items in batches. Returns (success_count, fail_count)."""
        success = 0
        fail = 0
        for i in range(0, len(items), self.batch_size):
            batch = items[i:i + self.batch_size]
            if self.process(batch):
                success += len(batch)
                self.processed_count += len(batch)
            else:
                fail += len(batch)
        return success, fail

    def export_results(self, output_path: str) -> bool:
        """Export cached results to a JSON file at the given path."""
        try:
            path = Path(output_path)
            path.parent.mkdir(parents=True, exist_ok=True)
            with open(path, 'w') as f:
                json.dump(self._cache, f, indent=2)
            return True
        except OSError:
            return False

def helper(x: int) -> str:
    """Convert an integer to its string representation."""
    return str(x)

def another_helper(x: int, y: int) -> int:
    """Add two integers together and return the result."""
    return x + y

def compute_hash(data: str, algorithm: str = 'sha256') -> str:
    """Compute a cryptographic hash of the given data string."""
    h = hashlib.new(algorithm)
    h.update(data.encode('utf-8'))
    return h.hexdigest()

def load_config(path: str) -> Optional[Config]:
    """Load configuration from a JSON file. Returns None on failure."""
    try:
        with open(path) as f:
            data = json.load(f)
        return Config(**data)
    except (OSError, KeyError, TypeError):
        return None

MAX_SIZE = 100
VERSION = "1.0.0"
DEBUG_MODE = False
DEFAULT_ENCODING = "utf-8"
MAX_CACHE_ENTRIES = 10000
SUPPORTED_ALGORITHMS = ["sha256", "sha512", "md5"]
`)
	return sb.String()
}

// bigJsTsSource creates a TS source large enough to exceed minTokensForStructure=1000.
// Uses varied content to avoid triggering the generated-file heuristics.
func bigJsTsSource() string {
	return `// TypeScript user service module
// Provides comprehensive user management utilities for the ctxgate system.
// This module implements the core service layer for interacting with the API.

import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Observable, throwError, of } from 'rxjs';
import { catchError, map, retry, tap } from 'rxjs/operators';

export interface UserData {
  id: number;
  name: string;
  email: string;
  role: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface PaginatedResult<T> {
  data: T[];
  total: number;
  page: number;
  pageSize: number;
}

export interface ServiceConfig {
  baseUrl: string;
  timeout: number;
  retries: number;
  debug: boolean;
}

export type UserId = number;
export type UserRole = 'admin' | 'user' | 'viewer';

export enum UserStatus {
  Active = 'active',
  Inactive = 'inactive',
  Pending = 'pending',
  Suspended = 'suspended',
}

export class UserService {
  private baseUrl: string;
  private headers: HttpHeaders;
  private requestCount: number = 0;
  private errorCount: number = 0;

  constructor(
    private http: HttpClient,
    private config: ServiceConfig,
  ) {
    this.baseUrl = config.baseUrl || 'https://api.example.com';
    this.headers = new HttpHeaders({
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    });
  }

  getUser(id: UserId): Observable<UserData> {
    this.requestCount++;
    return this.http.get<UserData>(
      this.baseUrl + '/users/' + id,
      { headers: this.headers }
    ).pipe(
      retry(this.config.retries),
      tap(data => {
        if (this.config.debug) {
          console.log('Fetched user:', data);
        }
      }),
      catchError(err => {
        this.errorCount++;
        return throwError(() => err);
      })
    );
  }

  updateUser(id: UserId, data: Partial<UserData>): Observable<UserData> {
    this.requestCount++;
    return this.http.put<UserData>(
      this.baseUrl + '/users/' + id,
      data,
      { headers: this.headers }
    ).pipe(
      catchError(err => {
        this.errorCount++;
        return throwError(() => err);
      })
    );
  }

  deleteUser(id: UserId): Observable<void> {
    this.requestCount++;
    return this.http.delete<void>(
      this.baseUrl + '/users/' + id,
      { headers: this.headers }
    );
  }

  listUsers(page: number = 1, pageSize: number = 20): Observable<PaginatedResult<UserData>> {
    const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
    return this.http.get<PaginatedResult<UserData>>(
      this.baseUrl + '/users?' + params.toString(),
      { headers: this.headers }
    );
  }

  getStats(): { requests: number; errors: number } {
    return { requests: this.requestCount, errors: this.errorCount };
  }

  resetStats(): void {
    this.requestCount = 0;
    this.errorCount = 0;
  }
}

export class AdminUserService extends UserService {
  constructor(http: HttpClient, config: ServiceConfig) {
    super(http, config);
  }

  promoteToAdmin(id: UserId): Observable<UserData> {
    return this.updateUser(id, { role: 'admin' });
  }

  suspendUser(id: UserId): Observable<UserData> {
    return this.updateUser(id, { role: 'viewer' });
  }

  bulkDelete(ids: UserId[]): Observable<void[]> {
    return new Observable(observer => {
      const promises = ids.map(id => this.deleteUser(id).toPromise());
      Promise.all(promises).then(
        results => { observer.next(results as void[]); observer.complete(); },
        err => observer.error(err)
      );
    });
  }
}

export function createService(http: HttpClient, config: Partial<ServiceConfig> = {}): UserService {
  const fullConfig: ServiceConfig = {
    baseUrl: 'https://api.example.com',
    timeout: 30000,
    retries: 3,
    debug: false,
    ...config,
  };
  return new UserService(http, fullConfig);
}

export function createAdminService(http: HttpClient, baseUrl: string): AdminUserService {
  return new AdminUserService(http, {
    baseUrl,
    timeout: 60000,
    retries: 5,
    debug: true,
  });
}

export const DEFAULT_CONFIG: ServiceConfig = {
  baseUrl: 'https://api.example.com',
  timeout: 30000,
  retries: 3,
  debug: false,
};

export const MAX_PAGE_SIZE = 100;
export const MIN_PAGE_SIZE = 1;
`
}

// ---------------------------------------------------------------------------
// 3. SummarizeCodeSource_Python_BelowMinTokens
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Python_BelowMinTokens(t *testing.T) {
	// Small Python file (< 1000 tokens ~= < 4000 chars)
	src := `
import os

def foo():
    return 1
`
	result := SummarizeCodeSource(src, "small.py", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "below_min_tokens", result.Reason)
	assert.Equal(t, "digest", result.ReplacementType)
	assert.Equal(t, "python", result.Language)
}

// ---------------------------------------------------------------------------
// 4. SummarizeCodeSource_Python_Structure
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Python_Structure(t *testing.T) {
	src := bigPythonSource()
	result := SummarizeCodeSource(src, "service.py", 0, 0, 0, 0)

	require.True(t, result.Eligible, "expected eligible=true, got reason=%q", result.Reason)

	validTypes := map[string]bool{"skeleton": true, "top_level": true, "signatures": true}
	assert.True(t, validTypes[result.ReplacementType], "unexpected type: %s", result.ReplacementType)

	assert.Contains(t, result.ReplacementText, "MyService")
	assert.Equal(t, "python", result.Language)
	assert.Greater(t, result.Confidence, 0.5)
	assert.NotEmpty(t, result.Fingerprint)
	assert.Greater(t, result.LineCount, 0)
	assert.NotNil(t, result.FileTokensEst)
	assert.Greater(t, *result.FileTokensEst, 0)
}

// ---------------------------------------------------------------------------
// 5. SummarizeCodeSource_Python_PartialRange
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Python_PartialRange(t *testing.T) {
	src := bigPythonSource()
	result := SummarizeCodeSource(src, "service.py", 10, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "partial_range_not_supported", result.Reason)
	assert.Equal(t, "digest", result.ReplacementType)
}

// ---------------------------------------------------------------------------
// 6. SummarizeCodeSource_Python_Generated
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Python_Generated(t *testing.T) {
	// Source starting with generated marker — needs to be big enough
	genBase := "# Generated by foo auto-generator\n# do not edit\n\n"
	// pad to exceed min tokens
	for i := 0; i < 300; i++ {
		genBase += "x_var_" + strings.Repeat("a", 10) + " = " + strings.Repeat("b", 10) + "\n"
	}
	result := SummarizeCodeSource(genBase, "generated.py", 0, 0, 0, 0)
	assert.True(t, result.GeneratedLike)
	assert.False(t, result.Eligible)
	assert.Equal(t, "generated_like", result.Reason)
}

// ---------------------------------------------------------------------------
// 7. SummarizeCodeSource_JsTs_ExportFunction
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_JsTs_ExportFunction(t *testing.T) {
	src := bigJsTsSource()
	result := SummarizeCodeSource(src, "service.ts", 0, 0, 0, 0)

	require.True(t, result.Eligible, "expected eligible=true, got reason=%q type=%q", result.Reason, result.ReplacementType)
	assert.Contains(t, result.ReplacementText, "createService")
	assert.Equal(t, "typescript", result.Language)
	assert.Greater(t, result.Confidence, 0.5)
}

// ---------------------------------------------------------------------------
// 8. SummarizeCodeSource_JsTs_Interface
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_JsTs_Interface(t *testing.T) {
	src := bigJsTsSource()
	result := SummarizeCodeSource(src, "service.ts", 0, 0, 0, 0)

	require.True(t, result.Eligible, "expected eligible=true, got reason=%q", result.Reason)
	assert.Contains(t, result.ReplacementText, "UserData")
}

// ---------------------------------------------------------------------------
// 9. SummarizeCodeSource_Markdown
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Markdown(t *testing.T) {
	src := `# Getting Started

## Installation

### Prerequisites

Install Go 1.24.

## Usage

### Basic Usage

Run the binary.

#### Advanced

See docs.
`
	result := SummarizeCodeSource(src, "README.md", 0, 0, 0, 0)
	assert.True(t, result.Eligible)
	assert.Equal(t, "outline", result.ReplacementType)
	assert.Equal(t, "markdown", result.Language)
	assert.Contains(t, result.ReplacementText, "Getting Started")
	assert.Contains(t, result.ReplacementText, "Installation")
}

// ---------------------------------------------------------------------------
// 10. SummarizeCodeSource_Markdown_NoHeadings
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Markdown_NoHeadings(t *testing.T) {
	src := "This is just plain text.\nNo headings here.\nJust paragraphs.\n"
	result := SummarizeCodeSource(src, "plain.md", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "no_headings", result.Reason)
}

// ---------------------------------------------------------------------------
// 11. SummarizeCodeSource_JSON
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_JSON(t *testing.T) {
	src := `{
  "name": "ctxgate",
  "version": "1.0.0",
  "description": "Fast Go hook runtime",
  "dependencies": {
    "cobra": "v1.8.1",
    "testify": "v1.11.1"
  },
  "scripts": {
    "build": "make build",
    "test": "make test"
  }
}`
	result := SummarizeCodeSource(src, "package.json", 0, 0, 0, 0)
	assert.True(t, result.Eligible)
	assert.Equal(t, "key_tree", result.ReplacementType)
	assert.Equal(t, "json", result.Language)
	assert.Contains(t, result.ReplacementText, "JSON structure for")
}

// ---------------------------------------------------------------------------
// 12. SummarizeCodeSource_YAML
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_YAML(t *testing.T) {
	src := `name: ctxgate
version: 1.0.0
services:
  dev:
    build: .
    volumes:
      - .:/app
dependencies:
  cobra: v1.8.1
  testify: v1.11.1
`
	result := SummarizeCodeSource(src, "docker-compose.yaml", 0, 0, 0, 0)
	assert.True(t, result.Eligible)
	assert.Equal(t, "key_tree", result.ReplacementType)
	assert.Equal(t, "yaml", result.Language)
	assert.Contains(t, result.ReplacementText, "YAML key tree for")
}

// ---------------------------------------------------------------------------
// 13. SummarizeCodeSource_TOML
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_TOML(t *testing.T) {
	src := `[package]
name = "ctxgate"
version = "0.1.0"
edition = "2021"

[dependencies]
cobra = "1.8.1"
testify = "1.11.1"

[build]
target = "release"
`
	result := SummarizeCodeSource(src, "Cargo.toml", 0, 0, 0, 0)
	assert.True(t, result.Eligible)
	assert.Equal(t, "section_list", result.ReplacementType)
	assert.Equal(t, "toml", result.Language)
	assert.Contains(t, result.ReplacementText, "TOML structure for")
	assert.Contains(t, result.ReplacementText, "[package]")
}

// ---------------------------------------------------------------------------
// 14. SummarizeCodeSource_Unsupported
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Unsupported(t *testing.T) {
	src := `package main

func main() {
	println("hello")
}
`
	result := SummarizeCodeSource(src, "main.go", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "unsupported_language", result.Reason)
	assert.Equal(t, "digest", result.ReplacementType)
}

// ---------------------------------------------------------------------------
// 15. SummarizeCodeFile_NotFound
// ---------------------------------------------------------------------------

func TestSummarizeCodeFile_NotFound(t *testing.T) {
	result := SummarizeCodeFile("/nonexistent/path/that/does/not/exist.py")
	assert.False(t, result.Eligible)
	assert.Equal(t, "unreadable", result.Reason)
	assert.Equal(t, "digest", result.ReplacementType)
}

// ---------------------------------------------------------------------------
// 16. TestFingerprint_Deterministic
// ---------------------------------------------------------------------------

func TestFingerprint_Deterministic(t *testing.T) {
	fp1 := fingerprint("/foo/bar.py", "skeleton", "python skeleton\nlines: 42", 42, 1234)
	fp2 := fingerprint("/foo/bar.py", "skeleton", "python skeleton\nlines: 42", 42, 1234)
	assert.Equal(t, fp1, fp2, "fingerprint should be deterministic")

	fp3 := fingerprint("/foo/bar.py", "signatures", "python skeleton\nlines: 42", 42, 1234)
	assert.NotEqual(t, fp1, fp3, "different type should produce different fingerprint")

	fp4 := fingerprint("/foo/bar.py", "skeleton", "python skeleton\nlines: 43", 43, 1234)
	assert.NotEqual(t, fp1, fp4, "different content should produce different fingerprint")
}

// ---------------------------------------------------------------------------
// 17. TestGeneratedDetection_Python
// ---------------------------------------------------------------------------

func TestGeneratedDetection_Python(t *testing.T) {
	// Source with "do not edit" marker
	src := "# do not edit — this file is auto-generated\n"
	for i := 0; i < 300; i++ {
		src += "some_var_" + strings.Repeat("x", 5) + " = " + strings.Repeat("y", 5) + "\n"
	}
	assert.True(t, looksGeneratedPython(src), "should detect 'do not edit' as generated")

	// Source with repeated lines heuristic
	var sb strings.Builder
	sb.WriteString("import os\n")
	line := "x = 1\n"
	for i := 0; i < 100; i++ {
		sb.WriteString(line)
	}
	assert.True(t, looksGeneratedPython(sb.String()), "high repeated ratio should be detected as generated")

	// Normal source should not be detected
	assert.False(t, looksGeneratedPython(bigPythonSource()))
}

// ---------------------------------------------------------------------------
// 18. TestGeneratedDetection_JsTs
// ---------------------------------------------------------------------------

func TestGeneratedDetection_JsTs(t *testing.T) {
	// Source with "auto-generated" marker
	src := "// auto-generated by build tool\n// do not modify\n\n"
	for i := 0; i < 200; i++ {
		src += "const varName = 'some value';\n"
	}
	assert.True(t, looksGeneratedJsTs(src), "should detect 'auto-generated' as generated")

	// Source with repeated lines heuristic
	var sb strings.Builder
	sb.WriteString("import { Component } from '@angular/core';\n")
	line := "const x = 1;\n"
	for i := 0; i < 100; i++ {
		sb.WriteString(line)
	}
	assert.True(t, looksGeneratedJsTs(sb.String()), "high repeated ratio should be detected as generated")

	// Normal source should not be detected
	assert.False(t, looksGeneratedJsTs(bigJsTsSource()))
}

// ---------------------------------------------------------------------------
// Additional edge-case tests
// ---------------------------------------------------------------------------

func TestSummarizeCodeSource_Python_EmptyFile(t *testing.T) {
	result := SummarizeCodeSource("   \n  \t  ", "empty.py", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "empty_file", result.Reason)
}

func TestSummarizeCodeSource_JsTs_BelowMinTokens(t *testing.T) {
	src := "export function foo() { return 1; }\n"
	result := SummarizeCodeSource(src, "foo.ts", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "below_min_tokens", result.Reason)
}

func TestSummarizeCodeSource_JsTs_PartialRange(t *testing.T) {
	src := bigJsTsSource()
	result := SummarizeCodeSource(src, "service.ts", 5, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "partial_range_not_supported", result.Reason)
}

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, estimateTokens(""))
	assert.Equal(t, 1, estimateTokens("abc"))   // 3/4 → ceil=1
	assert.Equal(t, 1, estimateTokens("abcd"))  // 4/4 = 1
	assert.Equal(t, 2, estimateTokens("abcde")) // 5/4 → ceil=2
	assert.Equal(t, 25, estimateTokens(strings.Repeat("x", 100))) // 100/4 = 25
}

func TestCountLines(t *testing.T) {
	assert.Equal(t, 0, countLines(""))
	assert.Equal(t, 1, countLines("hello"))
	assert.Equal(t, 2, countLines("hello\nworld"))
	assert.Equal(t, 3, countLines("a\nb\nc"))
}

func TestSummarizeCodeSource_Markdown_TruncatedHeadings(t *testing.T) {
	// Generate more than maxMDHeadings (20) headings
	var sb strings.Builder
	for i := 0; i < 25; i++ {
		sb.WriteString("# Heading ")
		sb.WriteString(strings.Repeat("A", 10))
		sb.WriteString("\n\nSome content here.\n\n")
	}
	result := SummarizeCodeSource(sb.String(), "long.md", 0, 0, 0, 0)
	assert.True(t, result.Eligible)
	assert.Contains(t, result.ReplacementText, "truncated at 20 headings")
}

func TestSummarizeCodeSource_JSON_TooLarge(t *testing.T) {
	// JSON larger than maxJSONBytes (100KB)
	bigJSON := `{"key":"` + strings.Repeat("x", 102400) + `"}`
	result := SummarizeCodeSource(bigJSON, "big.json", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "too_large_to_parse", result.Reason)
}

func TestSummarizeCodeSource_JSON_Invalid(t *testing.T) {
	result := SummarizeCodeSource("{not valid json}", "bad.json", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "parse_error", result.Reason)
}

func TestSummarizeCodeSource_YAML_NoKeys(t *testing.T) {
	result := SummarizeCodeSource("# just a comment\n# another comment\n", "empty.yaml", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	assert.Equal(t, "no_keys", result.Reason)
}

func TestSummarizeCodeSource_TOML_NoStructure(t *testing.T) {
	result := SummarizeCodeSource("# just a comment\n", "empty.toml", 0, 0, 0, 0)
	assert.False(t, result.Eligible)
	// comment-only file has no sections or keys → no_structure
	assert.Equal(t, "no_structure", result.Reason)
}

func TestDedupeStrings(t *testing.T) {
	items := []string{"a", "b", "a", "c", "b"}
	result := dedupeStrings(items)
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestNormalizeJsTsSignature_Truncation(t *testing.T) {
	long := strings.Repeat("x", 200)
	result := normalizeJsTsSignature(long, "")
	assert.LessOrEqual(t, len([]rune(result)), maxSignatureLen)
	assert.True(t, strings.HasSuffix(result, "…"))
}

func TestStripSignaturePrefix(t *testing.T) {
	assert.Equal(t, "foo()", stripSignaturePrefix("def foo()"))
	assert.Equal(t, "async foo()", stripSignaturePrefix("async def foo()"))
	assert.Equal(t, "class Foo", stripSignaturePrefix("class Foo"))
}
