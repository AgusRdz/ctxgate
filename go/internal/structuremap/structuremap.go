// Package structuremap extracts structural summaries from code files.
// It ports the Python structure_map.py using regex + line-scanning instead of ast.
package structuremap

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// StructureMapResult holds the output of structural code summarization.
type StructureMapResult struct {
	FilePath             string
	Language             string
	ReplacementType      string // "signatures" | "top_level" | "skeleton" | "digest"
	ReplacementText      string
	ReplacementTokensEst int
	Confidence           float64
	Fingerprint          string
	Eligible             bool
	Reason               string
	GeneratedLike        bool
	ParseOK              bool
	LineCount            int
	FileTokensEst        *int
	FileSizeBytes        *int
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	minTokensForStructure       = 1000
	maxASTBytes                 = 800 * 1024
	maxASTLines                 = 20_000
	maxJsTsBytes                = 400 * 1024
	maxJsTsLines                = 5_000
	maxMDHeadings               = 20
	maxMDHeadingChars           = 100
	maxJSONBytes                = 100 * 1024
	maxJSONKeys                 = 30
	maxNonCodeKeys              = 30
	maxDigestHeadLines          = 6
	maxDigestTailLines          = 6
	maxDocstringChars           = 120
	maxDecorators               = 2
	maxBases                    = 3
	maxSignatureItems           = 12
	maxTopLevelImports          = 10
	maxTopLevelSymbols          = 12
	maxSkeletonClasses          = 4
	maxSkeletonMethodsPerClass  = 4
	maxSkeletonTopLevelFunctions = 8
	maxSkeletonTopLevelImports  = 8
	maxSkeletonTopLevelAssigns  = 8
	maxJsTsImports              = 3
	maxJsTsSignatureItems       = 6
	maxJsTsTopLevelSymbols      = 8
	maxJsTsSkeletonFunctions    = 5
	maxJsTsSkeletonSymbols      = 4
	maxSignatureLen             = 120
)

var maxReplacementChars = map[string]int{
	"signatures": 800, "top_level": 1200, "skeleton": 2400, "digest": 900,
}
var maxReplacementCharsNonCode = map[string]int{
	"outline": 800, "key_tree": 800, "section_list": 600,
}

// ---------------------------------------------------------------------------
// Language detection
// ---------------------------------------------------------------------------

var languageLabels = map[string]string{
	".py": "python", ".js": "javascript", ".jsx": "javascript",
	".mjs": "javascript", ".cjs": "javascript",
	".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".md": "markdown", ".json": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "toml",
}

var jsTsSuffixes = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
}

var nonCodeSuffixes = map[string]bool{
	".md": true, ".json": true, ".yaml": true, ".yml": true, ".toml": true,
}

// IsStructureSupported returns true if filePath's extension has a structure extractor.
func IsStructureSupported(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ext == ".py" || jsTsSuffixes[ext] || nonCodeSuffixes[ext]
}

// DetectLanguage returns the language identifier for filePath based on extension.
func DetectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, ok := languageLabels[ext]; ok {
		return lang
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// SummarizeCodeFile reads filePath from disk and extracts structural information.
func SummarizeCodeFile(filePath string) StructureMapResult {
	content, err := os.ReadFile(filePath)
	if err != nil {
		lang := DetectLanguage(filePath)
		return buildFallbackResult(filePath, lang, "", "unreadable", 0.05, false, false, nil, nil)
	}
	src := string(content)
	sizeBytes := len(content)
	tokEst := estimateTokens(src)
	return SummarizeCodeSource(src, filePath, 0, 0, tokEst, sizeBytes)
}

// SummarizeCodeSource extracts structural information from content without reading disk.
// fileTokensEst and fileSizeBytes of 0 mean "compute it".
func SummarizeCodeSource(content, filePath string, offset, limit, fileTokensEst, fileSizeBytes int) StructureMapResult {
	ext := strings.ToLower(filepath.Ext(filePath))
	lang := DetectLanguage(filePath)

	if fileSizeBytes == 0 {
		fileSizeBytes = len([]byte(content))
	}
	if fileTokensEst == 0 {
		fileTokensEst = estimateTokens(content)
	}

	switch {
	case ext == ".py":
		return summarizePythonSource(content, filePath, offset, limit, fileTokensEst, fileSizeBytes)
	case jsTsSuffixes[ext]:
		return summarizeJsTsSource(content, filePath, offset, limit, fileTokensEst, fileSizeBytes)
	case nonCodeSuffixes[ext]:
		return summarizeNonCodeSource(content, filePath, ext, offset, limit, fileTokensEst, fileSizeBytes)
	default:
		return buildFallbackResult(filePath, lang, content, "unsupported_language", 0.10, false, false, &fileTokensEst, &fileSizeBytes)
	}
}

// ---------------------------------------------------------------------------
// Generated-file detection
// ---------------------------------------------------------------------------

var generatedMarkers = []string{
	"generated by", "auto-generated", "autogenerated", "do not edit",
	"this file was generated", "code generated", "generated file", "generated code",
}

func looksGeneratedPython(source string) bool {
	end := 4000
	if len(source) < end {
		end = len(source)
	}
	lower := strings.ToLower(source[:end])
	for _, marker := range generatedMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	allLines := strings.Split(source, "\n")
	var lines []string
	for _, l := range allLines {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 20 {
		return false
	}

	uniqueLines := make(map[string]struct{}, len(lines))
	for _, l := range lines {
		uniqueLines[l] = struct{}{}
	}
	repeatedRatio := 1.0 - float64(len(uniqueLines))/float64(len(lines))

	totalLen := 0
	longLines := 0
	commentLines := 0
	for _, l := range lines {
		totalLen += len(l)
		if len(l) > 240 {
			longLines++
		}
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			commentLines++
		}
	}
	averageLen := float64(totalLen) / float64(len(lines))
	longLineRatio := float64(longLines) / float64(len(lines))
	commentRatio := float64(commentLines) / float64(len(lines))

	if repeatedRatio >= 0.35 && len(lines) >= 80 {
		return true
	}
	if averageLen >= 200 && longLineRatio >= 0.30 {
		return true
	}
	if len(lines) >= 150 && commentRatio <= 0.05 && averageLen >= 120 && repeatedRatio >= 0.20 {
		return true
	}
	return false
}

func looksGeneratedJsTs(source string) bool {
	end := 5000
	if len(source) < end {
		end = len(source)
	}
	lower := strings.ToLower(source[:end])
	for _, marker := range generatedMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	allLines := strings.Split(source, "\n")
	var lines []string
	for _, l := range allLines {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 20 {
		return false
	}

	uniqueLines := make(map[string]struct{}, len(lines))
	for _, l := range lines {
		uniqueLines[l] = struct{}{}
	}
	repeatedRatio := 1.0 - float64(len(uniqueLines))/float64(len(lines))

	totalLen := 0
	longLines := 0
	for _, l := range lines {
		totalLen += len(l)
		if len(l) > 260 {
			longLines++
		}
	}
	averageLen := float64(totalLen) / float64(len(lines))
	longLineRatio := float64(longLines) / float64(len(lines))

	if repeatedRatio >= 0.35 && len(lines) >= 80 {
		return true
	}
	if averageLen >= 220 && longLineRatio >= 0.30 {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Python structure extraction (regex-based)
// ---------------------------------------------------------------------------

type pyFunc struct {
	name       string
	signature  string
	lineno     int
	decorators []string
	isAsync    bool
}

type pyClass struct {
	name       string
	signature  string
	lineno     int
	decorators []string
	methods    []pyFunc
}

var (
	pyImportRE      = regexp.MustCompile(`^import\s+(.+)$`)
	pyFromImportRE  = regexp.MustCompile(`^from\s+(\S+)\s+import\s+(.+)$`)
	pyClassRE       = regexp.MustCompile(`^class\s+([A-Za-z_]\w*)\s*(\([^)]*\))?\s*:`)
	pyDefRE         = regexp.MustCompile(`^(async\s+)?def\s+([A-Za-z_]\w*)\s*\(`)
	pyAllRE         = regexp.MustCompile(`^__all__\s*=\s*`)
	pyAllNamesRE    = regexp.MustCompile(`["']([A-Za-z_]\w*)["']`)
	pyUpperCaseRE   = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=`)
	pyDecoratorRE   = regexp.MustCompile(`^(@\S+)`)
	pyReturnTypeRE  = regexp.MustCompile(`\)\s*->\s*(.+?)\s*:\s*$`)
	pyDocstringRE   = regexp.MustCompile(`^\s*("""|\'\'\')`)
)

func summarizePythonSource(source, filePath string, offset, limit, fileTokensEst, fileSizeBytes int) StructureMapResult {
	lang := "python"
	lineCount := countLines(source)

	if offset != 0 || limit != 0 {
		return buildFallbackResult(filePath, lang, source, "partial_range_not_supported", 0.12, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if strings.TrimSpace(source) == "" {
		return buildFallbackResult(filePath, lang, source, "empty_file", 0.05, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if fileTokensEst < minTokensForStructure {
		return buildFallbackResult(filePath, lang, source, "below_min_tokens", 0.18, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if fileSizeBytes > maxASTBytes || lineCount > maxASTLines {
		return buildFallbackResult(filePath, lang, source, "over_ast_caps", 0.20, false, false, &fileTokensEst, &fileSizeBytes)
	}

	generatedLike := looksGeneratedPython(source)
	if generatedLike {
		return buildFallbackResult(filePath, lang, source, "generated_like", 0.16, false, true, &fileTokensEst, &fileSizeBytes)
	}

	// Parse with regex scanning
	imports, classes, functions, assignments := collectPythonStructure(source)
	parseOK := true

	moduleDocstring := extractPythonModuleDocstring(source)

	totalMethods := 0
	for _, cls := range classes {
		totalMethods += len(cls.methods)
	}

	replacementType := choosePythonReplacementType(fileTokensEst, lineCount, len(classes), len(functions), len(assignments), totalMethods)

	rendered := renderPythonCandidate(replacementType, filePath, lineCount, moduleDocstring, imports, classes, functions, assignments)

	if len(rendered) > maxReplacementChars[replacementType] {
		rendered, replacementType = shrinkPythonOrFallback(filePath, lineCount, moduleDocstring, imports, classes, functions, assignments, replacementType)
	}

	if len(rendered) > maxReplacementChars[replacementType] {
		return buildFallbackResult(filePath, lang, source, "structure_over_cap", 0.22, parseOK, false, &fileTokensEst, &fileSizeBytes)
	}

	eligible := replacementType != "digest"
	reason := "ok"
	if !eligible {
		reason = "fallback_digest"
	}

	confidence := scorePythonConfidence(replacementType, lineCount, fileTokensEst, len(classes), len(functions), totalMethods, len(rendered), parseOK, false)
	fp := fingerprint(filePath, replacementType, rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath:             filePath,
		Language:             lang,
		ReplacementType:      replacementType,
		ReplacementText:      rendered,
		ReplacementTokensEst: tokEst,
		Confidence:           confidence,
		Fingerprint:          fp,
		Eligible:             eligible,
		Reason:               reason,
		GeneratedLike:        false,
		ParseOK:              parseOK,
		LineCount:            lineCount,
		FileTokensEst:        &fileTokensEst,
		FileSizeBytes:        &fileSizeBytes,
	}
}

// extractPythonModuleDocstring extracts the module-level docstring (first triple-quoted string).
func extractPythonModuleDocstring(source string) string {
	lines := strings.Split(source, "\n")
	// Skip shebang and coding comments, find first non-empty, non-comment line
	inDocstring := false
	var dsLines []string
	var dsDelim string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inDocstring {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
				dsDelim = trimmed[:3]
				rest := trimmed[3:]
				// Check if it closes on same line
				if idx := strings.Index(rest, dsDelim); idx >= 0 {
					return rest[:idx]
				}
				dsLines = append(dsLines, rest)
				inDocstring = true
			}
			break
		}
	}
	if inDocstring {
		for _, line := range lines[len(dsLines)+1:] {
			if idx := strings.Index(line, dsDelim); idx >= 0 {
				dsLines = append(dsLines, line[:idx])
				break
			}
			dsLines = append(dsLines, line)
		}
		return strings.Join(dsLines, " ")
	}
	return ""
}

// collectPythonStructure scans source lines and extracts imports, classes, functions, and assignments.
func collectPythonStructure(source string) (imports []string, classes []pyClass, functions []pyFunc, assignments []string) {
	lines := strings.Split(source, "\n")
	n := len(lines)

	var currentClass *pyClass
	var currentClassIndent int = -1

	// Helper: measure indent
	indentOf := func(line string) int {
		spaces := 0
		for _, c := range line {
			if c == ' ' {
				spaces++
			} else if c == '\t' {
				spaces += 4
			} else {
				break
			}
		}
		return spaces
	}

	// Helper: collect decorators for a def/class at lineIdx (look backwards)
	collectDecorators := func(lineIdx, indent int) []string {
		var decs []string
		for i := lineIdx - 1; i >= 0; i-- {
			prevLine := lines[i]
			trimmed := strings.TrimSpace(prevLine)
			if trimmed == "" {
				continue
			}
			if indentOf(prevLine) == indent && strings.HasPrefix(trimmed, "@") {
				m := pyDecoratorRE.FindStringSubmatch(trimmed)
				if m != nil {
					decs = append([]string{m[1]}, decs...)
				} else {
					decs = append([]string{trimmed}, decs...)
				}
			} else {
				break
			}
		}
		if len(decs) > maxDecorators {
			decs = decs[:maxDecorators]
		}
		return decs
	}

	// Helper: build function signature from lines starting at lineIdx
	buildFuncSignature := func(lineIdx int, name, owner string, isAsync bool) string {
		line := lines[lineIdx]
		trimmed := strings.TrimSpace(line)

		// Try to find closing paren on same or following lines
		combined := trimmed
		for i := lineIdx + 1; i < n && !strings.Contains(combined, ")"); i++ {
			combined += " " + strings.TrimSpace(lines[i])
			if i-lineIdx > 5 {
				break
			}
		}

		prefix := "def"
		if isAsync {
			prefix = "async def"
		}
		fullName := name
		if owner != "" {
			fullName = owner + "." + name
		}

		if idx := strings.Index(combined, ")"); idx >= 0 {
			// Extract args
			argsStart := strings.Index(combined, "(")
			if argsStart < 0 {
				argsStart = 0
			}
			args := combined[argsStart+1 : idx]
			// Check for return type
			rest := combined[idx+1:]
			retAnnotation := ""
			if m := pyReturnTypeRE.FindStringSubmatch(")" + rest + ":"); m != nil {
				retAnnotation = " -> " + strings.TrimSpace(m[1])
			}
			sig := fmt.Sprintf("%s %s(%s)%s", prefix, fullName, args, retAnnotation)
			if len(sig) > maxSignatureLen {
				sig = sig[:maxSignatureLen-1] + "…"
			}
			return sig
		}
		// Multi-line args fallback
		sig := fmt.Sprintf("%s %s(...)", prefix, fullName)
		if len(sig) > maxSignatureLen {
			sig = sig[:maxSignatureLen-1] + "…"
		}
		return sig
	}

	for i, rawLine := range lines {
		// Skip empty lines
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}

		indent := indentOf(rawLine)

		// Determine if we are still in the current class body
		if currentClass != nil {
			if indent <= currentClassIndent {
				// Save class and reset
				classes = append(classes, *currentClass)
				currentClass = nil
				currentClassIndent = -1
			}
		}

		if indent == 0 {
			// Top-level statements
			if pyImportRE.MatchString(trimmed) {
				imports = append(imports, trimmed)
			} else if pyFromImportRE.MatchString(trimmed) {
				imports = append(imports, trimmed)
			} else if m := pyClassRE.FindStringSubmatch(trimmed); m != nil {
				// Class definition
				className := m[1]
				basesStr := ""
				if len(m) > 2 && m[2] != "" {
					// m[2] is "(Base1, Base2)"
					inner := strings.Trim(m[2], "()")
					bases := splitAndTrim(inner, ",")
					if len(bases) > maxBases {
						bases = bases[:maxBases]
					}
					if len(bases) > 0 && bases[0] != "" {
						basesStr = strings.Join(bases, ",")
					}
				}
				decs := collectDecorators(i, 0)
				sig := buildClassSignature(className, basesStr, decs)
				currentClass = &pyClass{
					name:       className,
					signature:  sig,
					lineno:     i + 1,
					decorators: decs,
				}
				currentClassIndent = 0
			} else if m := pyDefRE.FindStringSubmatch(trimmed); m != nil {
				// Top-level function
				isAsync := strings.TrimSpace(m[1]) == "async"
				funcName := m[2]
				decs := collectDecorators(i, 0)
				sig := buildFuncSignature(i, funcName, "", isAsync)
				functions = append(functions, pyFunc{
					name:       funcName,
					signature:  sig,
					lineno:     i + 1,
					decorators: decs,
					isAsync:    isAsync,
				})
			} else if pyAllRE.MatchString(trimmed) {
				// __all__ = [...]
				// Collect names from the rest of the line
				rest := pyAllRE.ReplaceAllString(trimmed, "")
				names := pyAllNamesRE.FindAllStringSubmatch(rest, -1)
				if len(names) > 0 {
					nameStrs := make([]string, 0, len(names))
					for _, nm := range names {
						nameStrs = append(nameStrs, nm[1])
					}
					assignments = append(assignments, "__all__ = "+strings.Join(nameStrs, ", "))
				} else {
					assignments = append(assignments, "__all__")
				}
			} else if m := pyUpperCaseRE.FindStringSubmatch(trimmed); m != nil {
				// UPPER_CASE assignment
				assignments = append(assignments, m[1])
			}
		} else if currentClass != nil {
			// Inside a class body — detect method at class_indent+4 (or any non-zero indent within class)
			// We consider anything at exactly class_indent+4 or class_indent+tab as method level
			expectedMethodIndent := currentClassIndent + 4
			if indent == expectedMethodIndent || (indent > currentClassIndent && indent < currentClassIndent+8) {
				if m := pyDefRE.FindStringSubmatch(trimmed); m != nil {
					isAsync := strings.TrimSpace(m[1]) == "async"
					methodName := m[2]
					sig := buildFuncSignature(i, methodName, currentClass.name, isAsync)
					currentClass.methods = append(currentClass.methods, pyFunc{
						name:      methodName,
						signature: sig,
						lineno:    i + 1,
						isAsync:   isAsync,
					})
				}
			}
		}
	}

	// Flush pending class
	if currentClass != nil {
		classes = append(classes, *currentClass)
	}

	return imports, classes, functions, assignments
}

func buildClassSignature(name, basesStr string, decs []string) string {
	var parts []string
	if len(decs) > 0 {
		parts = append(parts, "decorators="+strings.Join(decs, ","))
	}
	if basesStr != "" {
		parts = append(parts, "bases="+basesStr)
	}
	if len(parts) > 0 {
		return "class " + name + " (" + strings.Join(parts, "; ") + ")"
	}
	return "class " + name
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// choosePythonReplacementType selects the replacement type.
func choosePythonReplacementType(fileTokensEst, lineCount, classCount, funcCount, assignCount, methodCount int) string {
	totalSymbols := classCount + funcCount + assignCount
	hasClasses := classCount > 0
	hasMethods := methodCount > 0
	largefile := fileTokensEst >= 3500 || lineCount >= 900 || totalSymbols >= 18
	moderateFile := fileTokensEst >= 1500 || lineCount >= 220 || totalSymbols >= 8

	if hasClasses && hasMethods && moderateFile {
		return "skeleton"
	}
	if largefile {
		return "signatures"
	}
	if totalSymbols <= 6 && lineCount <= 180 {
		return "top_level"
	}
	if hasClasses && moderateFile {
		return "skeleton"
	}
	if totalSymbols <= 12 {
		return "top_level"
	}
	return "signatures"
}

func renderPythonCandidate(replacementType, path string, lineCount int, docstring string, imports []string, classes []pyClass, functions []pyFunc, assignments []string) string {
	sections := []string{"python " + replacementType, "lines: " + strconv.Itoa(lineCount)}
	if doc := shortDocstring(docstring); doc != "" {
		sections = append(sections, "docstring: "+doc)
	}

	switch replacementType {
	case "skeleton":
		sections = append(sections, renderPythonSkeleton(imports, classes, functions, assignments)...)
	case "top_level":
		sections = append(sections, renderPythonTopLevel(imports, classes, functions, assignments)...)
	case "signatures":
		sections = append(sections, renderPythonSignatures(imports, classes, functions, assignments)...)
	default:
		sections = append(sections, renderDigestLines(path, lineCount)...)
	}

	var nonEmpty []string
	for _, s := range sections {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

func renderPythonSkeleton(imports []string, classes []pyClass, functions []pyFunc, assignments []string) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxSkeletonTopLevelImports) {
			lines = append(lines, "  - "+item)
		}
	}
	if len(classes) > 0 {
		lines = append(lines, fmt.Sprintf("classes (%d):", len(classes)))
		for _, cls := range shortenListClasses(classes, maxSkeletonClasses) {
			lines = append(lines, fmt.Sprintf("  - %s @ L%d", cls.signature, cls.lineno))
			if len(cls.methods) > 0 {
				lines = append(lines, "    methods:")
				for _, method := range shortenListFuncs(cls.methods, maxSkeletonMethodsPerClass) {
					lines = append(lines, fmt.Sprintf("      - %s @ L%d", stripSignaturePrefix(method.signature), method.lineno))
				}
			}
		}
	}
	if len(functions) > 0 {
		lines = append(lines, fmt.Sprintf("functions (%d):", len(functions)))
		for _, fn := range shortenListFuncs(functions, maxSkeletonTopLevelFunctions) {
			lines = append(lines, fmt.Sprintf("  - %s @ L%d", fn.signature, fn.lineno))
		}
	}
	if len(assignments) > 0 {
		lines = append(lines, fmt.Sprintf("assignments (%d):", len(assignments)))
		for _, item := range shortenList(assignments, maxSkeletonTopLevelAssigns) {
			lines = append(lines, "  - "+item)
		}
	}
	return lines
}

func renderPythonTopLevel(imports []string, classes []pyClass, functions []pyFunc, assignments []string) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxTopLevelImports) {
			lines = append(lines, "  - "+item)
		}
	}
	var symbolLines []string
	for _, cls := range classes {
		symbolLines = append(symbolLines, fmt.Sprintf("%s @ L%d", cls.signature, cls.lineno))
	}
	for _, fn := range functions {
		symbolLines = append(symbolLines, fmt.Sprintf("%s @ L%d", fn.signature, fn.lineno))
	}
	for _, item := range assignments {
		symbolLines = append(symbolLines, "assign "+item)
	}
	if len(symbolLines) > 0 {
		lines = append(lines, fmt.Sprintf("symbols (%d):", len(symbolLines)))
		for _, item := range shortenList(symbolLines, maxTopLevelSymbols) {
			lines = append(lines, "  - "+item)
		}
	}
	return lines
}

func renderPythonSignatures(imports []string, classes []pyClass, functions []pyFunc, assignments []string) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxSkeletonTopLevelImports) {
			lines = append(lines, "  - "+item)
		}
	}
	var items []string
	for _, cls := range classes {
		items = append(items, fmt.Sprintf("%s @ L%d", cls.signature, cls.lineno))
		for _, method := range cls.methods {
			items = append(items, fmt.Sprintf("%s @ L%d", stripSignaturePrefix(method.signature), method.lineno))
		}
	}
	for _, fn := range functions {
		items = append(items, fmt.Sprintf("%s @ L%d", fn.signature, fn.lineno))
	}
	for _, item := range assignments {
		items = append(items, "assign "+item)
	}
	if len(items) > 0 {
		lines = append(lines, fmt.Sprintf("signatures (%d):", len(items)))
		for _, item := range shortenList(items, maxSignatureItems) {
			lines = append(lines, "  - "+item)
		}
	}
	return lines
}

func shrinkPythonOrFallback(path string, lineCount int, docstring string, imports []string, classes []pyClass, functions []pyFunc, assignments []string, preferred string) (string, string) {
	orderMap := map[string][]string{
		"skeleton":   {"skeleton", "top_level", "signatures", "digest"},
		"top_level":  {"top_level", "signatures", "digest"},
		"signatures": {"signatures", "top_level", "digest"},
		"digest":     {"digest"},
	}
	order, ok := orderMap[preferred]
	if !ok {
		order = []string{"digest"}
	}

	for _, rtype := range order {
		rendered := renderPythonCandidate(rtype, path, lineCount, docstring, imports, classes, functions, assignments)
		if len(rendered) <= maxReplacementChars[rtype] {
			return rendered, rtype
		}
	}

	rendered := strings.Join(renderDigestLines(path, lineCount), "\n")
	return rendered, "digest"
}

func scorePythonConfidence(replacementType string, lineCount, fileTokensEst, classCount, funcCount, methodCount, renderedLen int, parseOK, generatedLike bool) float64 {
	baseMap := map[string]float64{"skeleton": 0.90, "top_level": 0.94, "signatures": 0.86, "digest": 0.28}
	base, ok := baseMap[replacementType]
	if !ok {
		base = 0.35
	}
	if parseOK {
		base += 0.02
	}
	if generatedLike {
		base -= 0.20
	}
	if fileTokensEst >= 4000 {
		base -= 0.05
	}
	if lineCount >= 1200 {
		base -= 0.05
	}
	if classCount >= 3 {
		base -= 0.03
	}
	if methodCount >= 8 {
		base -= 0.04
	}
	if funcCount >= 12 {
		base -= 0.03
	}
	if cap, ok := maxReplacementChars[replacementType]; ok && renderedLen >= cap {
		base -= 0.04
	}
	return math.Round(math.Max(0.05, math.Min(0.98, base))*1000) / 1000
}

// ---------------------------------------------------------------------------
// JS/TS structure extraction
// ---------------------------------------------------------------------------

type jsTsFunc struct {
	name      string
	signature string
	lineno    int
	isAsync   bool
}

type jsTsClass struct {
	name      string
	signature string
	lineno    int
	methods   []jsTsFunc
}

type jsTsSymbol struct {
	kind      string
	name      string
	signature string
	lineno    int
	exported  bool
}

var (
	jsTsImportRE      = regexp.MustCompile(`^(?:import\b.*|export\s+\{[^}]+\}\s+from\b.*|export\s+\*\s+from\b.*)$`)
	jsTsClassRE       = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?(?:abstract\s+)?)class\s+(?P<name>[A-Za-z_$][\w$]*)`)
	jsTsInterfaceRE   = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?)interface\s+(?P<name>[A-Za-z_$][\w$]*)`)
	jsTsTypeRE        = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?)type\s+(?P<name>[A-Za-z_$][\w$]*)\b`)
	jsTsEnumRE        = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?)enum\s+(?P<name>[A-Za-z_$][\w$]*)\b`)
	jsTsFuncRE        = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?)(?P<async>async\s+)?function\s+(?P<name>[A-Za-z_$][\w$]*)\s*\(`)
	jsTsVarFuncRE     = regexp.MustCompile(`^(?P<prefix>(?:export\s+)?(?:default\s+)?)(?P<kind>const|let|var)\s+(?P<name>[A-Za-z_$][\w$]*)(?P<type>\s*:\s*[^=]+)?\s*=\s*(?:(?P<async>async)\s+)?(?:(?:function\b)|(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>)`)
	jsTsExportDefaultRE = regexp.MustCompile(`^export\s+default\s+(?P<name>[A-Za-z_$][\w$]*)\s*;?$`)
	jsTsExportListRE  = regexp.MustCompile(`^export\s+\{(?P<body>[^}]+)\}(?:\s+from\b.*)?$`)
	jsTsMethodRE      = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|async|abstract|override|get|set)\s+)*(?P<name>[A-Za-z_$][\w$]*)\s*\(`)
	jsTsPropertyArrowRE = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|async|override)\s+)*(?P<name>[A-Za-z_$][\w$]*)(?:\??\s*:\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)
	jsTsImportFromRE  = regexp.MustCompile(`\sfrom\s+(['"][^'"]+['"])`)
	jsTsBraceImportRE = regexp.MustCompile(`import\s+\{(?P<body>[^}]+)\}`)
	jsTsExportListBodyRE = regexp.MustCompile(`^export\s+\{(?P<body>[^}]+)\}`)
)

var jsTsMethodSkip = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true, "constructor": true,
}

func summarizeJsTsSource(source, filePath string, offset, limit, fileTokensEst, fileSizeBytes int) StructureMapResult {
	lang := DetectLanguage(filePath)
	lineCount := countLines(source)

	if offset != 0 || limit != 0 {
		return buildFallbackResult(filePath, lang, source, "partial_range_not_supported", 0.12, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if strings.TrimSpace(source) == "" {
		return buildFallbackResult(filePath, lang, source, "empty_file", 0.05, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if fileTokensEst < minTokensForStructure {
		return buildFallbackResult(filePath, lang, source, "below_min_tokens", 0.20, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if fileSizeBytes > maxJsTsBytes || lineCount > maxJsTsLines {
		return buildFallbackResult(filePath, lang, source, "over_ast_caps", 0.22, false, false, &fileTokensEst, &fileSizeBytes)
	}

	generatedLike := looksGeneratedJsTs(source)
	if generatedLike {
		return buildFallbackResult(filePath, lang, source, "generated_like", 0.18, false, true, &fileTokensEst, &fileSizeBytes)
	}

	imports, classes, functions, symbols := collectJsTsStructure(source)
	if len(imports) == 0 && len(classes) == 0 && len(functions) == 0 && len(symbols) == 0 {
		return buildFallbackResult(filePath, lang, source, "low_signal_structure", 0.24, false, false, &fileTokensEst, &fileSizeBytes)
	}

	totalMethods := 0
	for _, cls := range classes {
		totalMethods += len(cls.methods)
	}

	replacementType := chooseJsTsReplacementType(fileTokensEst, lineCount, len(classes), len(functions), len(symbols), totalMethods)
	rendered := renderJsTsCandidate(lang, replacementType, lineCount, imports, classes, functions, symbols)

	if len(rendered) > maxReplacementChars[replacementType] {
		rendered, replacementType = shrinkJsTsOrFallback(lang, lineCount, imports, classes, functions, symbols, replacementType)
	}

	if len(rendered) > maxReplacementChars[replacementType] {
		return buildFallbackResult(filePath, lang, source, "structure_over_cap", 0.24, false, false, &fileTokensEst, &fileSizeBytes)
	}

	eligible := replacementType != "digest"
	reason := "ok"
	if !eligible {
		reason = "fallback_digest"
	}

	confidence := scoreJsTsConfidence(replacementType, lineCount, fileTokensEst, len(classes), len(functions), len(symbols), totalMethods, len(rendered))
	fp := fingerprint(filePath, replacementType, rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath:             filePath,
		Language:             lang,
		ReplacementType:      replacementType,
		ReplacementText:      rendered,
		ReplacementTokensEst: tokEst,
		Confidence:           confidence,
		Fingerprint:          fp,
		Eligible:             eligible,
		Reason:               reason,
		GeneratedLike:        false,
		ParseOK:              false,
		LineCount:            lineCount,
		FileTokensEst:        &fileTokensEst,
		FileSizeBytes:        &fileSizeBytes,
	}
}

// stripJsTsCommentsAndStrings removes JS/TS comments and string literals, preserving line structure.
func stripJsTsCommentsAndStrings(source string) string {
	const (
		stateNormal      = 0
		stateLineComment = 1
		stateBlockComment = 2
		stateSingleQuote = 3
		stateDoubleQuote = 4
		stateTemplate    = 5
	)

	state := stateNormal
	escaped := false
	out := make([]byte, 0, len(source))

	for i := 0; i < len(source); i++ {
		ch := source[i]
		var nxt byte
		if i+1 < len(source) {
			nxt = source[i+1]
		}

		switch state {
		case stateLineComment:
			if ch == '\n' {
				state = stateNormal
				out = append(out, '\n')
			} else {
				out = append(out, ' ')
			}
			continue

		case stateBlockComment:
			if ch == '*' && nxt == '/' {
				out = append(out, ' ', ' ')
				state = stateNormal
				i++ // skip '/'
				continue
			}
			if ch == '\n' {
				out = append(out, '\n')
			} else {
				out = append(out, ' ')
			}
			continue

		case stateSingleQuote, stateDoubleQuote, stateTemplate:
			if ch == '\n' && state != stateTemplate {
				state = stateNormal
				out = append(out, '\n')
				escaped = false
				continue
			}
			if escaped {
				out = append(out, ' ')
				escaped = false
				continue
			}
			if ch == '\\' {
				out = append(out, ' ')
				escaped = true
				continue
			}
			if (state == stateSingleQuote && ch == '\'') ||
				(state == stateDoubleQuote && ch == '"') ||
				(state == stateTemplate && ch == '`') {
				state = stateNormal
				out = append(out, ' ')
				continue
			}
			if ch == '\n' {
				out = append(out, '\n')
			} else {
				out = append(out, ' ')
			}
			continue
		}

		// stateNormal
		if ch == '/' && nxt == '/' {
			out = append(out, ' ', ' ')
			state = stateLineComment
			i++ // skip second '/'
			continue
		}
		if ch == '/' && nxt == '*' {
			out = append(out, ' ', ' ')
			state = stateBlockComment
			i++ // skip '*'
			continue
		}
		if ch == '\'' {
			out = append(out, ' ')
			state = stateSingleQuote
			continue
		}
		if ch == '"' {
			out = append(out, ' ')
			state = stateDoubleQuote
			continue
		}
		if ch == '`' {
			out = append(out, ' ')
			state = stateTemplate
			continue
		}
		out = append(out, ch)
	}

	return string(out)
}

func collectJsTsStructure(source string) (imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol) {
	rawLines := strings.Split(source, "\n")
	cleanLines := strings.Split(stripJsTsCommentsAndStrings(source), "\n")

	depth := 0
	currentClassIndex := -1
	currentClassBodyDepth := -1

	for lineno := 0; lineno < len(rawLines) && lineno < len(cleanLines); lineno++ {
		rawLine := rawLines[lineno]
		cleanLine := cleanLines[lineno]
		clean := strings.TrimSpace(cleanLine)
		raw := strings.TrimSpace(rawLine)

		if clean == "" {
			opens := strings.Count(cleanLine, "{")
			closes := strings.Count(cleanLine, "}")
			depth = depth + opens - closes
			if depth < 0 {
				depth = 0
			}
			if currentClassIndex >= 0 && depth < currentClassBodyDepth {
				currentClassIndex = -1
				currentClassBodyDepth = -1
			}
			continue
		}

		if depth == 0 {
			if jsTsImportRE.MatchString(clean) {
				imports = append(imports, normalizeJsTsSignature(raw, ""))
			} else if cls := matchJsTsClass(clean, raw, lineno+1); cls != nil {
				classes = append(classes, *cls)
				currentClassIndex = len(classes) - 1
			} else if fn := matchJsTsFunction(clean, raw, lineno+1); fn != nil {
				functions = append(functions, *fn)
			} else if sym := matchJsTsSymbol(clean, raw, lineno+1); sym != nil {
				symbols = append(symbols, *sym)
			}
		} else if currentClassIndex >= 0 && depth == currentClassBodyDepth {
			if method := matchJsTsMethod(clean, raw, lineno+1, classes[currentClassIndex].name); method != nil {
				classes[currentClassIndex].methods = append(classes[currentClassIndex].methods, *method)
			}
		}

		// Update depth
		opens := strings.Count(cleanLine, "{")
		closes := strings.Count(cleanLine, "}")
		nextDepth := depth + opens - closes
		if nextDepth < 0 {
			nextDepth = 0
		}
		if currentClassIndex >= 0 && depth == 0 && nextDepth > depth {
			currentClassBodyDepth = nextDepth
		}
		if currentClassIndex >= 0 && nextDepth < currentClassBodyDepth {
			currentClassIndex = -1
			currentClassBodyDepth = -1
		}
		depth = nextDepth
	}

	// Dedupe imports (preserve order)
	imports = dedupeStrings(imports)

	// Dedupe symbols
	symbols = dedupeJsTsSymbols(symbols)

	return imports, classes, functions, symbols
}

func matchJsTsClass(clean, raw string, lineno int) *jsTsClass {
	m := jsTsClassRE.FindStringSubmatch(clean)
	if m == nil {
		return nil
	}
	nameIdx := jsTsClassRE.SubexpIndex("name")
	prefixIdx := jsTsClassRE.SubexpIndex("prefix")
	name := m[nameIdx]
	exported := strings.HasPrefix(strings.TrimSpace(m[prefixIdx]), "export") || strings.Contains(m[prefixIdx], "export")
	sig := normalizeJsTsSignature(raw, "class "+name)
	if exported && !strings.HasPrefix(sig, "export ") {
		sig = "export " + sig
	}
	return &jsTsClass{name: name, signature: sig, lineno: lineno}
}

func matchJsTsFunction(clean, raw string, lineno int) *jsTsFunc {
	m := jsTsFuncRE.FindStringSubmatch(clean)
	if m != nil {
		prefixIdx := jsTsFuncRE.SubexpIndex("prefix")
		nameIdx := jsTsFuncRE.SubexpIndex("name")
		asyncIdx := jsTsFuncRE.SubexpIndex("async")
		name := m[nameIdx]
		exported := strings.TrimSpace(m[prefixIdx]) != ""
		isAsync := strings.TrimSpace(m[asyncIdx]) != ""
		sig := normalizeJsTsSignature(raw, "function "+name+"(...)")
		if exported && !strings.HasPrefix(sig, "export ") {
			sig = "export " + sig
		}
		return &jsTsFunc{name: name, signature: sig, lineno: lineno, isAsync: isAsync}
	}

	m = jsTsVarFuncRE.FindStringSubmatch(clean)
	if m == nil {
		return nil
	}
	prefixIdx := jsTsVarFuncRE.SubexpIndex("prefix")
	kindIdx := jsTsVarFuncRE.SubexpIndex("kind")
	nameIdx := jsTsVarFuncRE.SubexpIndex("name")
	typeIdx := jsTsVarFuncRE.SubexpIndex("type")
	asyncIdx := jsTsVarFuncRE.SubexpIndex("async")

	name := m[nameIdx]
	exported := strings.TrimSpace(m[prefixIdx]) != ""
	isAsync := strings.TrimSpace(m[asyncIdx]) != ""
	prefix := m[kindIdx] + " " + name
	if m[typeIdx] != "" {
		typeStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m[typeIdx]), ":"))
		prefix += ": " + typeStr
	}
	if strings.Contains(clean, "=>") {
		prefix += " = (...) =>"
	} else {
		prefix += " = function(...)"
	}
	if exported {
		prefix = "export " + prefix
	}
	return &jsTsFunc{name: name, signature: prefix, lineno: lineno, isAsync: isAsync}
}

func matchJsTsSymbol(clean, raw string, lineno int) *jsTsSymbol {
	for _, entry := range []struct {
		re   *regexp.Regexp
		kind string
	}{
		{jsTsInterfaceRE, "interface"},
		{jsTsTypeRE, "type"},
		{jsTsEnumRE, "enum"},
	} {
		m := entry.re.FindStringSubmatch(clean)
		if m != nil {
			prefixIdx := entry.re.SubexpIndex("prefix")
			nameIdx := entry.re.SubexpIndex("name")
			name := m[nameIdx]
			exported := strings.TrimSpace(m[prefixIdx]) != ""
			sig := normalizeJsTsSignature(raw, entry.kind+" "+name)
			if exported && !strings.HasPrefix(sig, "export ") {
				sig = "export " + sig
			}
			return &jsTsSymbol{kind: entry.kind, name: name, signature: sig, lineno: lineno, exported: exported}
		}
	}

	m := jsTsExportDefaultRE.FindStringSubmatch(clean)
	if m != nil {
		nameIdx := jsTsExportDefaultRE.SubexpIndex("name")
		name := m[nameIdx]
		return &jsTsSymbol{kind: "export", name: name, signature: "export default " + name, lineno: lineno, exported: true}
	}

	m = jsTsExportListRE.FindStringSubmatch(clean)
	if m != nil {
		bodyIdx := jsTsExportListRE.SubexpIndex("body")
		body := m[bodyIdx]
		parts := splitAndTrim(body, ",")
		if len(parts) > 0 {
			joined := strings.Join(parts, ", ")
			return &jsTsSymbol{kind: "export", name: parts[0], signature: "export { " + joined + " }", lineno: lineno, exported: true}
		}
	}

	return nil
}

func matchJsTsMethod(clean, raw string, lineno int, owner string) *jsTsFunc {
	m := jsTsMethodRE.FindStringSubmatch(clean)
	if m != nil {
		nameIdx := jsTsMethodRE.SubexpIndex("name")
		name := m[nameIdx]
		if !jsTsMethodSkip[name] {
			sig := owner + "." + normalizeJsTsSignature(raw, name+"(...)")
			isAsync := strings.Contains(clean, "async ")
			return &jsTsFunc{name: name, signature: sig, lineno: lineno, isAsync: isAsync}
		}
	}

	m = jsTsPropertyArrowRE.FindStringSubmatch(clean)
	if m != nil {
		nameIdx := jsTsPropertyArrowRE.SubexpIndex("name")
		name := m[nameIdx]
		if !jsTsMethodSkip[name] {
			sig := owner + "." + name + " = (...) =>"
			isAsync := strings.Contains(clean, "async ")
			return &jsTsFunc{name: name, signature: sig, lineno: lineno, isAsync: isAsync}
		}
	}

	return nil
}

func normalizeJsTsSignature(raw, fallback string) string {
	compact := strings.TrimSpace(raw)
	compact = strings.TrimRight(compact, "{")
	compact = strings.TrimRight(compact, ";")
	compact = strings.Join(strings.Fields(compact), " ")
	if compact == "" {
		if fallback != "" {
			return fallback
		}
		return "..."
	}
	if strings.HasPrefix(compact, "import ") {
		compact = compressJsTsImport(compact)
	} else if strings.HasPrefix(compact, "export {") {
		compact = compressJsTsExportList(compact)
	}
	if len(compact) <= maxSignatureLen {
		return compact
	}
	// Truncate to 119 runes + ellipsis
	runes := []rune(compact)
	if len(runes) > maxSignatureLen {
		return string(runes[:maxSignatureLen-1]) + "…"
	}
	return compact
}

func compressJsTsImport(compact string) string {
	var module string
	if m := jsTsImportFromRE.FindStringSubmatch(compact); m != nil {
		module = " from " + m[1]
	}
	if strings.HasPrefix(compact, "import * as ") {
		left := compact
		if idx := strings.Index(compact, " from "); idx >= 0 {
			left = compact[:idx]
		}
		return left + module
	}
	if m := jsTsBraceImportRE.FindStringSubmatch(compact); m != nil {
		bodyIdx := jsTsBraceImportRE.SubexpIndex("body")
		names := splitAndTrim(m[bodyIdx], ",")
		preview := strings.Join(names[:min(3, len(names))], ", ")
		if len(names) > 3 {
			preview += ", …"
		}
		return "import { " + preview + " }" + module
	}
	return compact
}

func compressJsTsExportList(compact string) string {
	m := jsTsExportListBodyRE.FindStringSubmatch(compact)
	if m == nil {
		return compact
	}
	bodyIdx := jsTsExportListBodyRE.SubexpIndex("body")
	names := splitAndTrim(m[bodyIdx], ",")
	preview := strings.Join(names[:min(4, len(names))], ", ")
	if len(names) > 4 {
		preview += ", …"
	}
	suffix := ""
	if idx := strings.Index(compact, " from "); idx >= 0 {
		suffix = compact[idx:]
	}
	return "export { " + preview + " }" + suffix
}

func chooseJsTsReplacementType(fileTokensEst, lineCount, classCount, funcCount, symbolCount, methodCount int) string {
	totalSymbols := classCount + funcCount + symbolCount
	largefile := fileTokensEst >= 3200 || lineCount >= 850 || totalSymbols >= 18
	moderateFile := fileTokensEst >= 1500 || lineCount >= 220 || totalSymbols >= 8

	if classCount > 0 && methodCount > 0 && moderateFile {
		return "skeleton"
	}
	if largefile {
		return "signatures"
	}
	if totalSymbols <= 6 && lineCount <= 180 {
		return "top_level"
	}
	if classCount > 0 && moderateFile {
		return "skeleton"
	}
	if totalSymbols <= 12 {
		return "top_level"
	}
	return "signatures"
}

func renderJsTsCandidate(lang, replacementType string, lineCount int, imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol) string {
	sections := []string{lang + " " + replacementType, "lines: " + strconv.Itoa(lineCount)}

	switch replacementType {
	case "skeleton":
		sections = append(sections, renderJsTsSkeleton(imports, classes, functions, symbols)...)
	case "top_level":
		sections = append(sections, renderJsTsTopLevel(imports, classes, functions, symbols)...)
	case "signatures":
		sections = append(sections, renderJsTsSignatures(imports, classes, functions, symbols)...)
	default:
		sections = append(sections, renderDigestLines("memory", lineCount)...)
	}

	var nonEmpty []string
	for _, s := range sections {
		if s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	return strings.Join(nonEmpty, "\n")
}

func renderJsTsSkeleton(imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxJsTsImports) {
			lines = append(lines, "  - "+item)
		}
	}
	if len(classes) > 0 {
		lines = append(lines, fmt.Sprintf("classes (%d):", len(classes)))
		for _, cls := range shortenListJsTsClasses(classes, maxSkeletonClasses) {
			lines = append(lines, fmt.Sprintf("  - %s @ L%d", cls.signature, cls.lineno))
			if len(cls.methods) > 0 {
				lines = append(lines, "    methods:")
				for _, method := range shortenListJsTsFuncs(cls.methods, maxSkeletonMethodsPerClass) {
					lines = append(lines, fmt.Sprintf("      - %s @ L%d", stripJsTsSignaturePrefix(method.signature), method.lineno))
				}
			}
		}
	}
	if len(functions) > 0 {
		lines = append(lines, fmt.Sprintf("functions (%d):", len(functions)))
		for _, fn := range shortenListJsTsFuncs(functions, maxJsTsSkeletonFunctions) {
			lines = append(lines, fmt.Sprintf("  - %s @ L%d", fn.signature, fn.lineno))
		}
	}
	if len(symbols) > 0 {
		lines = append(lines, fmt.Sprintf("types/exports (%d):", len(symbols)))
		for _, sym := range shortenListJsTsSymbols(symbols, maxJsTsSkeletonSymbols) {
			lines = append(lines, fmt.Sprintf("  - %s @ L%d", sym.signature, sym.lineno))
		}
	}
	return lines
}

func renderJsTsTopLevel(imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxJsTsImports) {
			lines = append(lines, "  - "+item)
		}
	}
	var symbolLines []string
	for _, cls := range classes {
		symbolLines = append(symbolLines, fmt.Sprintf("%s @ L%d", cls.signature, cls.lineno))
	}
	for _, fn := range functions {
		symbolLines = append(symbolLines, fmt.Sprintf("%s @ L%d", fn.signature, fn.lineno))
	}
	for _, sym := range symbols {
		symbolLines = append(symbolLines, fmt.Sprintf("%s @ L%d", sym.signature, sym.lineno))
	}
	if len(symbolLines) > 0 {
		lines = append(lines, fmt.Sprintf("symbols (%d):", len(symbolLines)))
		for _, item := range shortenList(symbolLines, maxJsTsTopLevelSymbols) {
			lines = append(lines, "  - "+item)
		}
	}
	return lines
}

func renderJsTsSignatures(imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol) []string {
	var lines []string
	if len(imports) > 0 {
		lines = append(lines, fmt.Sprintf("imports (%d):", len(imports)))
		for _, item := range shortenList(imports, maxJsTsImports) {
			lines = append(lines, "  - "+item)
		}
	}
	var items []string
	for _, cls := range classes {
		items = append(items, fmt.Sprintf("%s @ L%d", cls.signature, cls.lineno))
		for _, method := range cls.methods {
			items = append(items, fmt.Sprintf("%s @ L%d", stripJsTsSignaturePrefix(method.signature), method.lineno))
		}
	}
	for _, fn := range functions {
		items = append(items, fmt.Sprintf("%s @ L%d", fn.signature, fn.lineno))
	}
	for _, sym := range symbols {
		items = append(items, fmt.Sprintf("%s @ L%d", sym.signature, sym.lineno))
	}
	if len(items) > 0 {
		lines = append(lines, fmt.Sprintf("signatures (%d):", len(items)))
		for _, item := range shortenList(items, maxJsTsSignatureItems) {
			lines = append(lines, "  - "+item)
		}
	}
	return lines
}

func shrinkJsTsOrFallback(lang string, lineCount int, imports []string, classes []jsTsClass, functions []jsTsFunc, symbols []jsTsSymbol, preferred string) (string, string) {
	orderMap := map[string][]string{
		"skeleton":   {"skeleton", "top_level", "signatures", "digest"},
		"top_level":  {"top_level", "signatures", "digest"},
		"signatures": {"signatures", "top_level", "digest"},
		"digest":     {"digest"},
	}
	order, ok := orderMap[preferred]
	if !ok {
		order = []string{"digest"}
	}
	for _, rtype := range order {
		rendered := renderJsTsCandidate(lang, rtype, lineCount, imports, classes, functions, symbols)
		if len(rendered) <= maxReplacementChars[rtype] {
			return rendered, rtype
		}
	}
	rendered := strings.Join(renderDigestLines("memory", lineCount), "\n")
	return rendered, "digest"
}

func scoreJsTsConfidence(replacementType string, lineCount, fileTokensEst, classCount, funcCount, symbolCount, methodCount, renderedLen int) float64 {
	baseMap := map[string]float64{"skeleton": 0.89, "top_level": 0.95, "signatures": 0.90, "digest": 0.30}
	base, ok := baseMap[replacementType]
	if !ok {
		base = 0.35
	}
	if fileTokensEst >= 4500 {
		base -= 0.02
	}
	if lineCount >= 1200 {
		base -= 0.02
	}
	if classCount >= 4 {
		base -= 0.02
	}
	if methodCount >= 10 {
		base -= 0.02
	}
	if funcCount >= 14 {
		base -= 0.02
	}
	if symbolCount >= 16 {
		base -= 0.01
	}
	if cap, ok := maxReplacementChars[replacementType]; ok && renderedLen >= cap {
		base -= 0.04
	}
	return math.Round(math.Max(0.05, math.Min(0.96, base))*1000) / 1000
}

func stripJsTsSignaturePrefix(sig string) string {
	// Strip owner prefix for method display (owner.method(...) -> method(...))
	// Actually in JS/TS we keep the owner in the signature already when recording
	// The Python version uses _strip_signature_prefix which removes "def " and "async def "
	// For JS/TS methods the signature already has owner.methodName(...)
	// The Python _strip_signature_prefix strips "def " prefix from method sigs
	return sig
}

// ---------------------------------------------------------------------------
// Non-code summaries
// ---------------------------------------------------------------------------

func summarizeNonCodeSource(source, filePath, suffix string, offset, limit, fileTokensEst, fileSizeBytes int) StructureMapResult {
	lang := languageLabels[suffix]
	lineCount := countLines(source)

	if offset != 0 || limit != 0 {
		return buildFallbackResult(filePath, lang, source, "partial_range_not_supported", 0.12, false, false, &fileTokensEst, &fileSizeBytes)
	}
	if strings.TrimSpace(source) == "" {
		return buildFallbackResult(filePath, lang, source, "empty_file", 0.05, false, false, &fileTokensEst, &fileSizeBytes)
	}

	switch suffix {
	case ".md":
		return summarizeMarkdown(source, filePath, lineCount, fileTokensEst, fileSizeBytes)
	case ".json":
		return summarizeJSON(source, filePath, lineCount, fileTokensEst, fileSizeBytes)
	case ".yaml", ".yml":
		return summarizeYAML(source, filePath, lineCount, fileTokensEst, fileSizeBytes)
	case ".toml":
		return summarizeTOML(source, filePath, lineCount, fileTokensEst, fileSizeBytes)
	}

	return buildFallbackResult(filePath, lang, source, "unsupported_language", 0.10, false, false, &fileTokensEst, &fileSizeBytes)
}

func summarizeMarkdown(source, filePath string, lineCount, fileTokensEst, fileSizeBytes int) StructureMapResult {
	lines := strings.Split(source, "\n")
	var headings []string
	for _, line := range lines {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "#") {
			level := 0
			for _, c := range stripped {
				if c == '#' {
					level++
				} else {
					break
				}
			}
			title := strings.TrimSpace(stripped[level:])
			if level >= 1 && level <= 4 && title != "" {
				indent := strings.Repeat("  ", level-1)
				if len(title) > maxMDHeadingChars {
					title = title[:maxMDHeadingChars]
				}
				headings = append(headings, indent+title)
			}
		}
		if len(headings) >= maxMDHeadings {
			break
		}
	}

	if len(headings) == 0 {
		return buildFallbackResult(filePath, "markdown", source, "no_headings", 0.15, false, false, &fileTokensEst, &fileSizeBytes)
	}

	name := filepath.Base(filePath)
	rendered := fmt.Sprintf("Markdown outline for %s (%d lines):\n", name, lineCount)
	rendered += strings.Join(headings, "\n")
	if len(headings) == maxMDHeadings {
		rendered += fmt.Sprintf("\n... (truncated at %d headings)", maxMDHeadings)
	}

	cap := maxReplacementCharsNonCode["outline"]
	if len(rendered) > cap {
		rendered = rendered[:cap-3] + "..."
	}

	fp := fingerprint(filePath, "outline", rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath: filePath, Language: "markdown",
		ReplacementType: "outline", ReplacementText: rendered,
		ReplacementTokensEst: tokEst, Confidence: 0.90, Fingerprint: fp,
		Eligible: true, Reason: "ok", GeneratedLike: false, ParseOK: true,
		LineCount: lineCount, FileTokensEst: &fileTokensEst, FileSizeBytes: &fileSizeBytes,
	}
}

func jsonTypeHint(val interface{}) string {
	if val == nil {
		return "null"
	}
	switch v := val.(type) {
	case map[string]interface{}:
		return fmt.Sprintf("object(%d keys)", len(v))
	case []interface{}:
		return fmt.Sprintf("array(%d)", len(v))
	case string:
		if len(v) > 40 {
			return fmt.Sprintf("string(%dch)", len(v))
		}
		return fmt.Sprintf("%q", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers are float64
		if v == math.Trunc(v) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func extractJSONKeys(data interface{}, depth, maxDepth, maxKeys int, result *[]string) {
	if depth > maxDepth || len(*result) >= maxKeys {
		return
	}
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if len(*result) >= maxKeys {
				break
			}
			indent := strings.Repeat("  ", depth)
			hint := jsonTypeHint(val)
			*result = append(*result, fmt.Sprintf("%s%s: %s", indent, key, hint))
			if m, ok := val.(map[string]interface{}); ok && depth < maxDepth {
				_ = m
				extractJSONKeys(val, depth+1, maxDepth, maxKeys-len(*result), result)
			}
		}
	case []interface{}:
		if len(v) > 0 {
			indent := strings.Repeat("  ", depth)
			*result = append(*result, fmt.Sprintf("%s[%d items, first: %s]", indent, len(v), jsonTypeHint(v[0])))
		}
	}
}

func summarizeJSON(source, filePath string, lineCount, fileTokensEst, fileSizeBytes int) StructureMapResult {
	rawBytes := len([]byte(source))
	if rawBytes > maxJSONBytes {
		return buildFallbackResult(filePath, "json", source, "too_large_to_parse", 0.10, false, false, &fileTokensEst, &fileSizeBytes)
	}
	var data interface{}
	if err := json.Unmarshal([]byte(source), &data); err != nil {
		return buildFallbackResult(filePath, "json", source, "parse_error", 0.10, false, false, &fileTokensEst, &fileSizeBytes)
	}

	var keys []string
	extractJSONKeys(data, 0, 2, maxJSONKeys, &keys)
	if len(keys) == 0 {
		return buildFallbackResult(filePath, "json", source, "no_keys", 0.15, true, false, &fileTokensEst, &fileSizeBytes)
	}

	name := filepath.Base(filePath)
	rendered := "JSON structure for " + name + ":\n" + strings.Join(keys, "\n")
	cap := maxReplacementCharsNonCode["key_tree"]
	if len(rendered) > cap {
		rendered = rendered[:cap-3] + "..."
	}

	fp := fingerprint(filePath, "key_tree", rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath: filePath, Language: "json",
		ReplacementType: "key_tree", ReplacementText: rendered,
		ReplacementTokensEst: tokEst, Confidence: 0.92, Fingerprint: fp,
		Eligible: true, Reason: "ok", GeneratedLike: false, ParseOK: true,
		LineCount: lineCount, FileTokensEst: &fileTokensEst, FileSizeBytes: &fileSizeBytes,
	}
}

func summarizeYAML(source, filePath string, lineCount, fileTokensEst, fileSizeBytes int) StructureMapResult {
	lines := strings.Split(source, "\n")
	indentUnit := 2
	for _, ln := range lines {
		if ln != "" && ln[0] == ' ' {
			spaces := len(ln) - len(strings.TrimLeft(ln, " "))
			if spaces > 0 {
				indentUnit = spaces
				break
			}
		}
	}

	var keys []string
	for _, line := range lines {
		stripped := strings.TrimRight(line, " \t\r")
		if stripped == "" || strings.TrimSpace(stripped) == "" {
			continue
		}
		trimmedLine := strings.TrimSpace(stripped)
		if strings.HasPrefix(trimmedLine, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		depth := indent / max(indentUnit, 1)
		if depth <= 2 && strings.Contains(stripped, ":") {
			keyPart := strings.Split(stripped, ":")[0]
			keyPart = strings.TrimSpace(keyPart)
			keyPart = strings.TrimLeft(keyPart, "- ")
			keyPart = strings.TrimSpace(keyPart)
			if keyPart != "" && !strings.HasPrefix(keyPart, "#") {
				keys = append(keys, strings.Repeat("  ", depth)+keyPart)
			}
		}
		if len(keys) >= maxNonCodeKeys {
			break
		}
	}

	if len(keys) == 0 {
		return buildFallbackResult(filePath, "yaml", source, "no_keys", 0.15, false, false, &fileTokensEst, &fileSizeBytes)
	}

	name := filepath.Base(filePath)
	rendered := fmt.Sprintf("YAML key tree for %s (%d lines):\n", name, lineCount) + strings.Join(keys, "\n")
	cap := maxReplacementCharsNonCode["key_tree"]
	if len(rendered) > cap {
		rendered = rendered[:cap-3] + "..."
	}

	fp := fingerprint(filePath, "key_tree", rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath: filePath, Language: "yaml",
		ReplacementType: "key_tree", ReplacementText: rendered,
		ReplacementTokensEst: tokEst, Confidence: 0.85, Fingerprint: fp,
		Eligible: true, Reason: "ok", GeneratedLike: false, ParseOK: true,
		LineCount: lineCount, FileTokensEst: &fileTokensEst, FileSizeBytes: &fileSizeBytes,
	}
}

func summarizeTOML(source, filePath string, lineCount, fileTokensEst, fileSizeBytes int) StructureMapResult {
	var sections []string
	var topKeys []string

	for _, line := range strings.Split(source, "\n") {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "[") && strings.HasSuffix(stripped, "]") {
			sections = append(sections, stripped)
		} else if strings.Contains(stripped, "=") && !strings.HasPrefix(stripped, "#") {
			key := strings.TrimSpace(strings.Split(stripped, "=")[0])
			if key != "" && len(topKeys) < 15 {
				topKeys = append(topKeys, key)
			}
		}
	}

	if len(sections) == 0 && len(topKeys) == 0 {
		return buildFallbackResult(filePath, "toml", source, "no_structure", 0.15, false, false, &fileTokensEst, &fileSizeBytes)
	}

	name := filepath.Base(filePath)
	parts := []string{fmt.Sprintf("TOML structure for %s (%d lines):", name, lineCount)}
	if len(sections) > 0 {
		parts = append(parts, "Sections:")
		limit := 15
		if len(sections) < limit {
			limit = len(sections)
		}
		for _, s := range sections[:limit] {
			parts = append(parts, "  "+s)
		}
		if len(sections) > 15 {
			parts = append(parts, fmt.Sprintf("  ... (%d more)", len(sections)-15))
		}
	}
	if len(topKeys) > 0 {
		parts = append(parts, "Top-level keys:")
		for _, k := range topKeys {
			parts = append(parts, "  "+k)
		}
	}

	rendered := strings.Join(parts, "\n")
	cap := maxReplacementCharsNonCode["section_list"]
	if len(rendered) > cap {
		rendered = rendered[:cap-3] + "..."
	}

	fp := fingerprint(filePath, "section_list", rendered, lineCount, fileSizeBytes)
	tokEst := estimateTokens(rendered)

	return StructureMapResult{
		FilePath: filePath, Language: "toml",
		ReplacementType: "section_list", ReplacementText: rendered,
		ReplacementTokensEst: tokEst, Confidence: 0.88, Fingerprint: fp,
		Eligible: true, Reason: "ok", GeneratedLike: false, ParseOK: true,
		LineCount: lineCount, FileTokensEst: &fileTokensEst, FileSizeBytes: &fileSizeBytes,
	}
}

// ---------------------------------------------------------------------------
// Digest / fallback
// ---------------------------------------------------------------------------

func renderDigest(source, filePath string, lineCount int) string {
	lines := strings.Split(source, "\n")
	name := filepath.Base(filePath)
	if name == "" {
		name = filePath
	}

	if len(lines) == 0 {
		return fmt.Sprintf("digest fallback for %s\nlines: 0", name)
	}

	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, strings.TrimRight(l, " \t\r"))
		}
	}

	head := nonEmpty
	if len(head) > maxDigestHeadLines {
		head = head[:maxDigestHeadLines]
	}
	var tail []string
	if len(nonEmpty) > maxDigestHeadLines+maxDigestTailLines {
		tail = nonEmpty[len(nonEmpty)-maxDigestTailLines:]
	}

	parts := []string{fmt.Sprintf("digest fallback for %s", name), fmt.Sprintf("lines: %d", lineCount)}
	if len(head) > 0 {
		parts = append(parts, "head:")
		for _, l := range head {
			line := l
			if len(line) > 160 {
				line = line[:160]
			}
			parts = append(parts, "  - "+line)
		}
	}
	if len(tail) > 0 {
		parts = append(parts, "tail:")
		for _, l := range tail {
			line := l
			if len(line) > 160 {
				line = line[:160]
			}
			parts = append(parts, "  - "+line)
		}
	}

	rendered := strings.Join(parts, "\n")
	cap := maxReplacementChars["digest"]
	if len(rendered) <= cap {
		return rendered
	}
	runes := []rune(rendered)
	if len(runes) > cap {
		return string(runes[:cap-1]) + "…"
	}
	return rendered
}

func renderDigestLines(path string, lineCount int) []string {
	name := filepath.Base(path)
	if name == "" {
		name = path
	}
	return []string{
		"digest fallback for " + name,
		"lines: " + strconv.Itoa(lineCount),
	}
}

func buildFallbackResult(filePath, language, source, reason string, confidence float64, parseOK, generatedLike bool, fileTokensEst, fileSizeBytes *int) StructureMapResult {
	lineCount := countLines(source)
	rendered := renderDigest(source, filePath, lineCount)

	var fsBytes int
	if fileSizeBytes != nil {
		fsBytes = *fileSizeBytes
	}

	fp := fingerprint(filePath, "digest", rendered, lineCount, fsBytes)
	tokEst := estimateTokens(rendered)
	conf := math.Max(0.05, math.Min(1.0, confidence))

	return StructureMapResult{
		FilePath:             filePath,
		Language:             language,
		ReplacementType:      "digest",
		ReplacementText:      rendered,
		ReplacementTokensEst: tokEst,
		Confidence:           conf,
		Fingerprint:          fp,
		Eligible:             false,
		Reason:               reason,
		GeneratedLike:        generatedLike,
		ParseOK:              parseOK,
		LineCount:            lineCount,
		FileTokensEst:        fileTokensEst,
		FileSizeBytes:        fileSizeBytes,
	}
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return max(1, int(math.Ceil(float64(len(text))/4.0)))
}

func countLines(source string) int {
	if source == "" {
		return 0
	}
	return strings.Count(source, "\n") + 1
}

func fingerprint(path, replacementType, rendered string, lineCount, fileSizeBytes int) string {
	payload := strings.Join([]string{
		"structure-map-v1", replacementType, path,
		strconv.Itoa(lineCount), strconv.Itoa(fileSizeBytes), rendered,
	}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum)
}

func shortDocstring(docstring string) string {
	if docstring == "" {
		return ""
	}
	compact := strings.Join(strings.Fields(strings.TrimSpace(docstring)), " ")
	if len(compact) <= maxDocstringChars {
		return compact
	}
	runes := []rune(compact)
	return string(runes[:maxDocstringChars-1]) + "…"
}

func stripSignaturePrefix(sig string) string {
	if strings.HasPrefix(sig, "async def ") {
		return "async " + sig[len("async def "):]
	}
	if strings.HasPrefix(sig, "def ") {
		return sig[len("def "):]
	}
	return sig
}

func shortenList(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortenListClasses(items []pyClass, limit int) []pyClass {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortenListFuncs(items []pyFunc, limit int) []pyFunc {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortenListJsTsClasses(items []jsTsClass, limit int) []jsTsClass {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortenListJsTsFuncs(items []jsTsFunc, limit int) []jsTsFunc {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortenListJsTsSymbols(items []jsTsSymbol, limit int) []jsTsSymbol {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func dedupeJsTsSymbols(symbols []jsTsSymbol) []jsTsSymbol {
	type key struct{ kind, name, sig string }
	seen := make(map[key]struct{}, len(symbols))
	result := make([]jsTsSymbol, 0, len(symbols))
	for _, sym := range symbols {
		k := key{sym.kind, sym.name, sym.signature}
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			result = append(result, sym)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
