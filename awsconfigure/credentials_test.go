package awsconfigure

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestSetINI(t *testing.T) {
	creds := []setting{{"aws_access_key_id", "KEY"}, {"aws_secret_access_key", "SECRET"}}
	tests := []struct {
		name, content, section, want string
	}{
		{
			name:    "empty file",
			section: "default",
			want:    "[default]\naws_access_key_id = KEY\naws_secret_access_key = SECRET\n",
		},
		{
			name:    "appends a missing section",
			content: "[other]\naws_access_key_id = OTHER\n",
			section: "default",
			want:    "[other]\naws_access_key_id = OTHER\n\n[default]\naws_access_key_id = KEY\naws_secret_access_key = SECRET\n",
		},
		{
			name:    "replaces keys in place and keeps the rest",
			content: "# comment\n[default]\naws_secret_access_key=OLD\nregion = us-east-1\n\n[other]\naws_access_key_id = OTHER\n",
			section: "default",
			want:    "# comment\n[default]\naws_secret_access_key = SECRET\nregion = us-east-1\naws_access_key_id = KEY\n\n[other]\naws_access_key_id = OTHER\n",
		},
		{
			name:    "matches a header with spaces",
			content: "[ deploy ]\naws_access_key_id = OLD\n",
			section: "deploy",
			want:    "[ deploy ]\naws_access_key_id = KEY\naws_secret_access_key = SECRET\n",
		},
		{
			name:    "does not touch the same key in another section",
			content: "[deploy]\naws_access_key_id = DEPLOY\n",
			section: "default",
			want:    "[deploy]\naws_access_key_id = DEPLOY\n\n[default]\naws_access_key_id = KEY\naws_secret_access_key = SECRET\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, setINI(tt.content, tt.section, creds), tt.want)
		})
	}
}
