package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/alingse/go-linter-runner/runner/utils"
)

var ErrSkipNoGoModRepo = errors.New("skip this repo for no go.mod file exists")

func runCmd(cmd *exec.Cmd) error {
	data, err := cmd.CombinedOutput()
	log.Printf("run cmd %+v got len(output)=%d and err %+v\n", cmd, len(data), err)
	fmt.Println(string(data))
	if err != nil {
		return fmt.Errorf("run %s %+v failed %w", cmd.Path, cmd.Args, err)
	}
	return nil
}

func Prepare(ctx context.Context, cfg *Config) error {
	// install linter
	name, args := utils.SplitCommand(cfg.LinterCfg.InstallCommand)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.LinterCfg.Workdir
	if err := runCmd(cmd); err != nil {
		return err
	}

	// clone repo
	cmd = exec.CommandContext(ctx, "rm", "-rf", cfg.RepoDir)
	cmd.Dir = cfg.LinterCfg.Workdir
	if err := runCmd(cmd); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, "git", "clone", cfg.Repo)
	cmd.Dir = cfg.LinterCfg.Workdir
	if err := runCmd(cmd); err != nil {
		return err
	}

	// TODO: check more deep
	// check go.mod exists
	gomodFile := path.Join(cfg.RepoDir, "go.mod")
	if !utils.IsFileExists(gomodFile) {
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
	cfg.RepoTarget = cfg.Repo + "/blob/" + cfg.RepoBranch
	return nil
}

func Build(ctx context.Context, cfg *Config) error {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = cfg.RepoDir
	if err := runCmd(cmd); err != nil {
		return err
	}
	return nil
}

func Run(ctx context.Context, cfg *Config) ([]string, error) {
	name, args := utils.SplitCommand(cfg.LinterCfg.LinterCommand)
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.RepoDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("run cmd %+v got err %+v \n", cmd, err)
		fmt.Printf("stdout:\n%s\n", stdout.String())
		fmt.Printf("stderr:\n%s\n", stderr.String())
	}
	output := stdout.String()
	if utils.CastToBool(cfg.LinterCfg.CollectStderr) {
		output = output + "\n" + stderr.String()
	}

	output = strings.TrimSpace(output)
	if len(output) == 0 {
		return nil, nil
	}

	// check includes && excludes
	outputs := strings.Split(output, "\n")
	validOutputs := make([]string, 0, len(outputs))

	includes := utils.GetStringArray(cfg.LinterCfg.Includes)
	excludes := utils.GetStringArray(cfg.LinterCfg.Excludes)
	for _, line := range outputs {
		line := strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if includeLine(includes, line) && !excludeLine(excludes, line) {
			validOutputs = append(validOutputs, line)
		}
	}
	return validOutputs, nil
}

func Parse(ctx context.Context, cfg *Config, outputs []string) []string {
	// replace local path to a github link
	for i, line := range outputs {
		if strings.Contains(line, cfg.RepoDir) {
			outputs[i] = strings.ReplaceAll(line, cfg.RepoDir, cfg.RepoTarget)
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
	fmt.Printf("Report issue: %s/issues\n", cfg.Repo)
}

func includeLine(includes []string, line string) bool {
	if len(includes) == 0 {
		return true
	}
	for _, v := range includes {
		if strings.Contains(line, v) {
			return true
		}
	}
	return false
}

func excludeLine(excludes []string, line string) bool {
	if len(excludes) == 0 {
		return false
	}
	for _, v := range excludes {
		if strings.Contains(line, v) {
			return true
		}
	}
	return false
}

func buildIssueComment(cfg *Config, outputs []string) string {
	var s strings.Builder
	s.WriteString(fmt.Sprintf("Run `%s` on Repo: %s\n", cfg.LinterCfg.LinterCommand, cfg.Repo))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("Got total %d line output in action: %s", len(outputs), os.Getenv("GH_ACTION_LINK")))
	s.WriteString("\n")
	s.WriteString("<details>\n")
	s.WriteString("<summary>Expand</summary>\n\n")
	for i, line := range outputs {
		text := buildIssueCommentLine(cfg, line)
		s.WriteString(fmt.Sprintf("%d. %s\n", i+1, text))
	}
	s.WriteString("\n")
	s.WriteString("</details>\n\n")
	s.WriteString(fmt.Sprintf("Report issue: %s/issues\n", cfg.Repo))
	return s.String()
}

func buildIssueCommentLine(cfg *Config, line string) string {
	codePath, other := buildIssueCommentLineSplit(cfg, line)
	if codePath == "" {
		return line
	}
	pathText := strings.TrimLeft(strings.ReplaceAll(codePath, cfg.RepoTarget, ""), "/:")
	return fmt.Sprintf("[%s](%s) %s", pathText, codePath, other)
}

func buildIssueCommentLineSplit(cfg *Config, line string) (codePath string, other string) {
	index := strings.Index(line, cfg.RepoTarget)
	if index < 0 {
		return "", line
	}
	other = line[:index]
	tail := line[index:]
	index = strings.Index(tail, " ")
	if index < 0 {
		codePath = tail
		return strings.TrimSpace(codePath), strings.TrimSpace(other)
	}
	codePath = tail[:index]
	other += tail[index:]
	return strings.TrimSpace(codePath), strings.TrimSpace(other)
}

func CreateIssueComment(ctx context.Context, cfg *Config, outputs []string) error {
	body := buildIssueComment(cfg, outputs)
	cmd := exec.CommandContext(ctx, "gh", "issue", "comment",
		cfg.LinterCfg.IssueID,
		"--body", body)
	cmd.Dir = "."
	log.Printf("comment on issue #%s\n", cfg.LinterCfg.IssueID)
	return runCmd(cmd)
}
