package structuremap

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

// IsStructureSupported returns true if filePath's extension has a structure extractor.
func IsStructureSupported(filePath string) bool {
	panic("not implemented")
}

// DetectLanguage returns the language identifier for filePath based on extension.
func DetectLanguage(filePath string) string {
	panic("not implemented")
}

// SummarizeCodeSource extracts structural information from content without reading disk.
func SummarizeCodeSource(content, filePath string, offset, limit, fileTokensEst, fileSizeBytes int) StructureMapResult {
	panic("not implemented")
}

// SummarizeCodeFile reads filePath from disk and extracts structural information.
func SummarizeCodeFile(filePath string) StructureMapResult {
	panic("not implemented")
}
