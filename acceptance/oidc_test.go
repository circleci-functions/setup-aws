package setupaws_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestCLI_OIDC_RequiresOrganizationID verifies that an empty
// $CIRCLE_ORGANIZATION_ID fails with a message naming it, rather than minting
// a token with an empty audience.
func TestCLI_OIDC_RequiresOrganizationID(t *testing.T) {
	stubDir := writeStubAWS(t, "2.34.60")
	record := filepath.Join(t.TempDir(), "circleci-calls")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\necho token\n", record)
	assert.NilError(t, os.WriteFile(filepath.Join(stubDir, "circleci"), []byte(script), 0750))
	_, run := sandbox(t)

	out, err := run(t, map[string]string{
		"PATH":                   stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER":              "disabled",
		"CIRCLE_JOB":             "build",
		"CIRCLE_ORGANIZATION_ID": "",
	}, "--configure-credentials", "--role-arn", "arn:aws:iam::123456789012:role/Deploy", "--region", "us-east-1")
	assert.Assert(t, err != nil, "expected an error, got: %s", out)
	assert.Assert(t, strings.Contains(out, "--role-arn needs $CIRCLE_ORGANIZATION_ID, the audience of the CircleCI OIDC token, and it is empty"), "got: %s", out)

	_, statErr := os.Stat(record)
	assert.Assert(t, os.IsNotExist(statErr), "circleci should not be called")
}
