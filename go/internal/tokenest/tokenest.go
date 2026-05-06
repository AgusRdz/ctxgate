package tokenest

const CharsPerToken = 4.0

// Estimate returns the estimated token count for text using CharsPerToken.
func Estimate(text string) int {
	return int(float64(len(text)) / CharsPerToken)
}

// EstimateBytes returns the estimated token count for a byte slice.
func EstimateBytes(b []byte) int {
	return int(float64(len(b)) / CharsPerToken)
}
