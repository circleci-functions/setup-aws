package setupaws_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/circleci-functions/toolkit/testing/compiler"
)

var binaryPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	ctx := context.Background()

	p := compiler.NewParallel(1)
	defer p.Cleanup()

	p.Add(compiler.Work{
		Result: &binaryPath,
		Name:   "setup-aws",
		Target: "..",
		Source: ".",
	})

	if err := p.Run(ctx); err != nil {
		return 0, err
	}

	return m.Run(), nil
}

func runWithEnv(t *testing.T, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binaryPath, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
