package tokenest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimate_Empty(t *testing.T) {
	require.Equal(t, 0, Estimate(""))
}

func TestEstimate_ExactMultiple(t *testing.T) {
	// 8 chars / 4.0 = 2 tokens
	require.Equal(t, 2, Estimate("abcdefgh"))
}

func TestEstimate_Truncates(t *testing.T) {
	// 5 chars / 4.0 = 1.25 → 1 (truncated, not rounded)
	require.Equal(t, 1, Estimate("abcde"))
}

func TestEstimate_LargeText(t *testing.T) {
	text := make([]byte, 4000)
	for i := range text {
		text[i] = 'x'
	}
	require.Equal(t, 1000, Estimate(string(text)))
}

func TestEstimateBytes_Empty(t *testing.T) {
	require.Equal(t, 0, EstimateBytes(nil))
}

func TestEstimateBytes_Simple(t *testing.T) {
	require.Equal(t, 2, EstimateBytes([]byte("abcdefgh")))
}

func TestEstimateBytes_ConsistentWithEstimate(t *testing.T) {
	text := "hello world, this is a test string"
	require.Equal(t, Estimate(text), EstimateBytes([]byte(text)))
}
