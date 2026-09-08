package setupaws_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
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
	p := compiler.NewParallel(1)
	defer p.Cleanup()

	p.Add(compiler.Work{
		Result: &binaryPath,
		Name:   "setup-aws",
		Target: "..",
		Source: ".",
	})

	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}

	return m.Run(), nil
}

// runFunc runs the compiled binary with extraEnv applied on top of the
// sandbox's environment, so entries in extraEnv take precedence.
type runFunc func(t *testing.T, extraEnv map[string]string, args ...string) (string, error)

// sandbox returns a sandbox plus a run function bound to its env. The sandbox
// points $HOME, $BASH_ENV and the cache at temp locations, so a run never
// touches the real ones.
func sandbox(t *testing.T) (*sdktest.Sandbox, runFunc) {
	t.Helper()
	s := sdktest.NewSandbox(t)
	return s, func(t *testing.T, extraEnv map[string]string, args ...string) (string, error) {
		t.Helper()
		r := runner.New(t, binaryPath, runner.WithSandboxEnv(sandboxEnv(s, extraEnv)))
		_, err := r.Run(args...)
		return r.StdoutString() + r.StderrString(), err
	}
}

// sandboxEnv returns the sandbox's environment with extra applied on top.
func sandboxEnv(s *sdktest.Sandbox, extra map[string]string) map[string]string {
	env := s.Env()
	maps.Copy(env, extra)
	return env
}
