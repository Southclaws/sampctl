package commands

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Southclaws/sampctl/src/config"
)

func TestShouldCheckForUpdates(t *testing.T) {
	t.Parallel()

	falseValue := false
	trueValue := true

	tests := []struct {
		name               string
		cfg                *config.Config
		generateCompletion bool
		bare               bool
		now                time.Time
		want               bool
	}{
		{
			name: "missing config",
			cfg:  nil,
			now:  time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name:               "bash completion generation",
			cfg:                &config.Config{HideVersionUpdateMessage: &falseValue},
			now:                time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
			generateCompletion: true,
			want:               false,
		},
		{
			name: "bare mode",
			cfg:  &config.Config{HideVersionUpdateMessage: &falseValue},
			now:  time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
			bare: true,
			want: false,
		},
		{
			name: "hidden update messages",
			cfg:  &config.Config{HideVersionUpdateMessage: &trueValue},
			now:  time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "odd minute",
			cfg:  &config.Config{HideVersionUpdateMessage: &falseValue},
			now:  time.Date(2026, time.March, 25, 12, 1, 0, 0, time.UTC),
			want: false,
		},
		{
			name: "odd second",
			cfg:  &config.Config{HideVersionUpdateMessage: &falseValue},
			now:  time.Date(2026, time.March, 25, 12, 0, 1, 0, time.UTC),
			want: false,
		},
		{
			name: "allowed",
			cfg:  &config.Config{HideVersionUpdateMessage: &falseValue},
			now:  time.Date(2026, time.March, 25, 12, 2, 2, 0, time.UTC),
			want: true,
		},
		{
			name: "nil hide flag defaults to allowed",
			cfg:  &config.Config{},
			now:  time.Date(2026, time.March, 25, 12, 2, 2, 0, time.UTC),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldCheckForUpdates(tt.cfg, tt.generateCompletion, tt.bare, tt.now)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNeedsUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
		wantErr bool
	}{
		{name: "newer release available", current: "1.0.0", latest: "1.1.0", want: true},
		{name: "same version", current: "1.1.0", latest: "1.1.0", want: false},
		{name: "older release", current: "1.2.0", latest: "1.1.0", want: false},
		{name: "invalid current", current: "master", latest: "1.1.0", wantErr: true},
		{name: "invalid latest", current: "1.0.0", latest: "banana", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := needsUpgrade(tt.current, tt.latest)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
