package cross

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	HelperFile   = "src/helper.ts"
	WidgetFile   = "src/widget.ts"
	UserFile     = "src/user.ts"
	ConsumerFile = "src/consumer.ts"
	FnGreet      = "greet"
	TypeOptions  = "Options"
	FieldVerbose = "verbose"
	MethodRun    = "Widget.run"
	ClassWidget  = "Widget"
	ClassHandler = "Handler"
	TypeOpts     = "Opts"
)

// Session runs Python and Go CLIs against one indexed fixture with an isolated cache (HOME).
type Session struct {
	FixtureDir   string
	HomeDir      string
	PythonBinary string
	GoBinary     string
}

func (s *Session) Env() []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "SCIP_CLI_CACHE=") || strings.HasPrefix(e, "HOME=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+s.HomeDir)
	return env
}

type CLIResult struct {
	Stdout string
	Stderr string
	Code   int
}

func (s *Session) run(binary string, args ...string) CLIResult {
	cmd := exec.Command(binary, args...)
	cmd.Dir = s.FixtureDir
	cmd.Env = s.Env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return CLIResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Code:   code,
	}
}

func (s *Session) RunPython(args ...string) CLIResult {
	return s.run(s.PythonBinary, args...)
}

func (s *Session) RunGo(args ...string) CLIResult {
	return s.run(s.GoBinary, args...)
}

func (s *Session) Reindex(binary string) error {
	cmd := exec.Command(binary, "reindex")
	cmd.Dir = s.FixtureDir
	cmd.Env = s.Env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s reindex: %w: %s", binary, err, out)
	}
	return nil
}

func ResolvePythonCLI(repoRoot string) (string, error) {
	if p, err := exec.LookPath("scip-cli"); err == nil {
		base := filepath.Base(p)
		if base != "scip-cli-go" && !strings.Contains(p, "scip-cli-go") {
			return p, nil
		}
	}
	sibling := filepath.Join(repoRoot, "..", "scip-cli", ".venv", "bin", "scip-cli")
	if _, err := os.Stat(sibling); err == nil {
		return sibling, nil
	}
	homeLocal := filepath.Join(os.Getenv("HOME"), ".local", "bin", "scip-cli")
	if _, err := os.Stat(homeLocal); err == nil {
		return homeLocal, nil
	}
	return "", fmt.Errorf("Python scip-cli not found on PATH or at ../scip-cli/.venv/bin/scip-cli")
}

func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
