package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProfile_ValidProfiles(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantLoop int
		wantInterval int
		wantSleep int
	}{
		{"conservative", "conservative", 3, 5, 3},
		{"moderate", "moderate", 1, 1, 1},
		{"aggressive", "aggressive", 1, 1, 1},
		{"extreme", "extreme", 1, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := GetProfile(tt.profile)
			require.NoError(t, err)
			assert.Equal(t, tt.wantLoop, p.Loop)
			assert.Equal(t, tt.wantInterval, p.Interval)
			assert.Equal(t, tt.wantSleep, p.Sleep)
		})
	}
}

func TestGetProfile_InvalidProfile(t *testing.T) {
	_, err := GetProfile("nonexistent")
	assert.Error(t, err)
}

func TestGetProfile_ExtremeDistinguishing(t *testing.T) {
	p, err := GetProfile("extreme")
	require.NoError(t, err)
	assert.Equal(t, 99, p.SysMemThreshold)
	assert.Equal(t, 2, p.SwapCacheHighWmark)
	assert.Equal(t, 1, p.SwapCacheLowWmark)
}

func TestValidProfileNames(t *testing.T) {
	names := ValidProfileNames()
	assert.Contains(t, names, "conservative")
	assert.Contains(t, names, "moderate")
	assert.Contains(t, names, "aggressive")
	assert.Contains(t, names, "extreme")
	assert.Len(t, names, 4)
}
