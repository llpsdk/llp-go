package llp

import "testing"

func TestPresenceStatus_String(t *testing.T) {
	tests := []struct {
		status   PresenceStatus
		expected string
	}{
		{Available, "available"},
		{Unavailable, "unavailable"},
		{PresenceStatus(99), "unavailable"}, // Unknown defaults to unavailable
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Errorf("PresenceStatus.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParsePresenceStatus(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  PresenceStatus
		shouldErr bool
	}{
		{
			name:      "available",
			input:     "available",
			expected:  Available,
			shouldErr: false,
		},
		{
			name:      "unavailable",
			input:     "unavailable",
			expected:  Unavailable,
			shouldErr: false,
		},
		{
			name:      "invalid",
			input:     "online",
			expected:  Unavailable,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePresenceStatus(tt.input)

			if tt.shouldErr {
				if err == nil {
					t.Error("ParsePresenceStatus() expected error, got nil")
				}
				if err != ErrInvalidStatus {
					t.Errorf("ParsePresenceStatus() error = %v, want %v", err, ErrInvalidStatus)
				}
			} else {
				if err != nil {
					t.Errorf("ParsePresenceStatus() unexpected error: %v", err)
				}
			}

			if got != tt.expected {
				t.Errorf("ParsePresenceStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}
