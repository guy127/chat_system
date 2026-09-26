package chat

import (
	"errors"
	"strings"
	"testing"

	"qrchat/internal/apperr"
)

func TestValidateBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string // "" = valid
	}{
		{"plain", "hello", ""},
		{"thai counts characters not bytes", strings.Repeat("ก", MaxBodyLen), ""},
		{"exactly max", strings.Repeat("a", MaxBodyLen), ""},
		{"too long", strings.Repeat("a", MaxBodyLen+1), apperr.MessageTooLong.Code},
		{"empty", "", "invalid_input"},
		{"whitespace only", " \n\t ", "invalid_input"},
		{"invalid utf8", "\xff\xfe", "invalid_input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBody(tt.body)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateBody() = %v, want nil", err)
				}
				return
			}
			var e *apperr.Error
			if !errors.As(err, &e) || e.Code != tt.wantCode {
				t.Fatalf("ValidateBody() = %v, want code %s", err, tt.wantCode)
			}
		})
	}
}
