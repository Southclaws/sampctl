package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectRuntimeType(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected RuntimeType
	}{
		{
			name:     "SA-MP version",
			version:  "0.3.7",
			expected: RuntimeTypeSAMP,
		},
		{
			name:     "SA-MP DL version",
			version:  "0.3DL",
			expected: RuntimeTypeSAMP,
		},
		{
			name:     "open.mp version lowercase",
			version:  "v1.0.0-openmp",
			expected: RuntimeTypeOpenMP,
		},
		{
			name:     "open.mp version uppercase",
			version:  "v1.0.0-OPENMP",
			expected: RuntimeTypeOpenMP,
		},
		{
			name:     "open.mp with dot notation",
			version:  "v1.0.0-open.mp",
			expected: RuntimeTypeOpenMP,
		},
		{
			name:     "Mixed case open.mp",
			version:  "v1.0.0-Open.MP",
			expected: RuntimeTypeOpenMP,
		},
		{
			name:     "Unknown version defaults to SA-MP",
			version:  "some-custom-version",
			expected: RuntimeTypeSAMP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectRuntimeType(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRuntimeGetEffectiveRuntimeType(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		explicitType   RuntimeType
		expectedResult RuntimeType
	}{
		{
			name:           "Auto-detect SA-MP",
			version:        "0.3.7",
			explicitType:   RuntimeTypeAuto,
			expectedResult: RuntimeTypeSAMP,
		},
		{
			name:           "Auto-detect open.mp",
			version:        "v1.0.0-openmp",
			explicitType:   RuntimeTypeAuto,
			expectedResult: RuntimeTypeOpenMP,
		},
		{
			name:           "Explicit SA-MP overrides auto-detection",
			version:        "v1.0.0-openmp",
			explicitType:   RuntimeTypeSAMP,
			expectedResult: RuntimeTypeSAMP,
		},
		{
			name:           "Explicit open.mp overrides auto-detection",
			version:        "0.3.7",
			explicitType:   RuntimeTypeOpenMP,
			expectedResult: RuntimeTypeOpenMP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := Runtime{
				Version:     tt.version,
				RuntimeType: tt.explicitType,
			}
			result := runtime.GetEffectiveRuntimeType()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestRuntimeIsOpenMP(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		explicitType RuntimeType
		expected     bool
	}{
		{
			name:         "SA-MP version",
			version:      "0.3.7",
			explicitType: RuntimeTypeAuto,
			expected:     false,
		},
		{
			name:         "open.mp version",
			version:      "v1.0.0-openmp",
			explicitType: RuntimeTypeAuto,
			expected:     true,
		},
		{
			name:         "Explicit open.mp",
			version:      "0.3.7",
			explicitType: RuntimeTypeOpenMP,
			expected:     true,
		},
		{
			name:         "Explicit SA-MP",
			version:      "v1.0.0-openmp",
			explicitType: RuntimeTypeSAMP,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := Runtime{
				Version:     tt.version,
				RuntimeType: tt.explicitType,
			}
			result := runtime.IsOpenMP()
			assert.Equal(t, tt.expected, result)
		})
	}
}
