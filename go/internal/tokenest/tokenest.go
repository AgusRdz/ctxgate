package tokenest

const CharsPerToken = 4.0

// Estimate returns the token count estimate for a string.
func Estimate(text string) int {
	return int(float64(len(text)) / CharsPerToken)
}

// EstimateBytes returns the token count estimate for a byte slice.
func EstimateBytes(b []byte) int {
	return int(float64(len(b)) / CharsPerToken)
}
