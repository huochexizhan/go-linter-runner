package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

var ErrSkipNoGoModRepo = errors.New("skip this repo for no go.mod file exists")

func runCmd(cmd *exec.Cmd) error {
	data, err := cmd.CombinedOutput()
	fmt.Println(string(data))
	if err != nil {
		return fmt.Errorf("run %s %+v failed %w", cmd.Path, cmd.Args, err)
	}
	return nil
}

func Prepare(ctx context.Context, cfg *Config) error {
	// install linter
	args := strings.Fields(cfg.LinterCfg.InstallCommand)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cfg.LinterCfg.Workdir
	if err := runCmd(cmd); err != nil {
		return err
	}

	// clone repo
	cmd = exec.CommandContext(ctx, "git", "clone", cfg.LinterCfg.RepoURL)
	cmd.Dir = cfg.LinterCfg.Workdir
	if err := runCmd(cmd); err != nil {
		return err
	}

	// TODO: check more deep
	// check go.mod exists
	gomodFile := path.Join(cfg.RepoDir, "go.mod")
	if !isFileExists(gomodFile) {
		return ErrSkipNoGoModRepo
	}

	// run go mod download
	cmd = exec.CommandContext(ctx, "go", "mod", "download")
	cmd.Dir = cfg.RepoDir
	if err := runCmd(cmd); err != nil {
		return err
	}

	// read default branch for repo
	cmd = exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = cfg.RepoDir
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git branch failed %w", err)
	}
	cfg.RepoBranch = strings.TrimSpace(string(output))
	return nil
}

func Run(ctx context.Context, cfg *Config) ([]string, error) {
	args := strings.Fields(cfg.LinterCfg.LinterCommand)
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, args[0], args...)
	cmd.Dir = cfg.RepoDir
	data, err := cmd.CombinedOutput()
	output := string(data)
	if err != nil && len(output) == 0 {
		return nil, err
	}

	// check includes && excludes
	outputs := strings.Split(output, "\n")
	validOutputs := make([]string, 0, len(outputs))
	for _, line := range outputs {
		line := strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if includeLine(cfg, line) && !excludeLine(cfg, line) {
			validOutputs = append(validOutputs, line)
		}
	}
	return validOutputs, nil
}

func Parse(ctx context.Context, cfg *Config, outputs []string) []string {
	target := cfg.LinterCfg.RepoURL + "/blob/" + cfg.RepoBranch
	// replace local path to a github link
	for i, line := range outputs {
		if strings.Contains(line, cfg.RepoDir) {
			outputs[i] = strings.ReplaceAll(line, cfg.RepoDir, target)
		}
	}

	// process the example.go:7:6 -> #L7
	for i, line := range outputs {
		if strings.Contains(line, ".go:") {
			outputs[i] = strings.ReplaceAll(line, ".go:", ".go#L")
		}
	}
	return outputs
}

var divider = strings.Repeat(`=`, 100)

func PrintOutput(ctx context.Context, cfg *Config, outputs []string) {
	fmt.Printf("Run linter `%s` got %d line outputs\n", cfg.LinterCfg.LinterCommand, len(outputs))
	fmt.Println(divider)
	fmt.Printf("runner config: %+v\n", cfg)
	fmt.Println(divider)
	for _, line := range outputs {
		fmt.Println(line)
	}
	fmt.Println(divider)
	fmt.Printf("Report issue: %s/issues\n", cfg.LinterCfg.RepoURL)
}

func isFileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}
	return true
}

func includeLine(c *Config, line string) bool {
	if len(c.LinterCfg.Includes) == 0 {
		return true
	}
	for _, v := range c.LinterCfg.Includes {
		if strings.Contains(line, v) {
			return true
		}
	}
	return false
}

func excludeLine(c *Config, line string) bool {
	if len(c.LinterCfg.Excludes) == 0 {
		return false
	}
	for _, v := range c.LinterCfg.Excludes {
		if strings.Contains(line, v) {
			return true
		}
	}
	return false
}

func buildIssueComment(cfg *Config, outputs []string) string {
	var s strings.Builder
	s.WriteString(fmt.Sprintf("Run `%s` on Repo: %s got output\n", cfg.LinterCfg.LinterCommand, cfg.LinterCfg.RepoURL))
	s.WriteString("```\n")
	for _, o := range outputs {
		s.WriteString(o)
		s.WriteString("\n")
	}
	s.WriteString("```\n")
	s.WriteString(fmt.Sprintf("Report issue: %s/issues\n", cfg.LinterCfg.RepoURL))
	s.WriteString(fmt.Sprintf("Github actions: %s", os.Getenv("GH_ACTION_LINK")))
	return s.String()
}

func CreateIssueComment(ctx context.Context, cfg *Config, outputs []string) error {
	body := buildIssueComment(cfg, outputs)
	cmd := exec.CommandContext(ctx, "gh", "issue", "comment",
		strconv.FormatInt(cfg.LinterCfg.IssueID, 10),
		"--body", body)
	cmd.Dir = "."
	log.Printf("comment on issue #%d\n", cfg.LinterCfg.IssueID)
	return runCmd(cmd)
}
