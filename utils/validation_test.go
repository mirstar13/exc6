package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{
			name:     "Valid username",
			username: "valid_user-123",
			wantErr:  false,
		},
		{
			name:     "Too short",
			username: "ab",
			wantErr:  true,
		},
		{
			name:     "Too long",
			username: "this_username_is_definitely_way_too_long_to_be_valid_in_our_system",
			wantErr:  true,
		},
		{
			name:     "Invalid characters",
			username: "user@name",
			wantErr:  true,
		},
		{
			name:     "Space not allowed",
			username: "user name",
			wantErr:  true,
		},
		{
			name:     "HTML tags",
			username: "<script>alert(1)</script>",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
