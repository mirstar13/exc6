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

func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
	}{
		{
			name:      "Valid group name",
			groupName: "Valid Group Name 123",
			wantErr:   false,
		},
		{
			name:      "Too short",
			groupName: "ab",
			wantErr:   true,
		},
		{
			name:      "Too long",
			groupName: "this group name is definitely way too long to be valid in our system because it exceeds fifty characters limit",
			wantErr:   true,
		},
		{
			name:      "Invalid characters",
			groupName: "Group <script>",
			wantErr:   true,
		},
		{
			name:      "Special characters",
			groupName: "Group @ Name",
			wantErr:   true,
		},
		{
			name:      "Hyphens and Underscores",
			groupName: "Group-Name_1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupName(tt.groupName)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
