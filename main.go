package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/alingse/go-linter-runner/runner"
)

func main() {
	var cfg, err = runner.LoadCfg()
	if err != nil {
		log.Fatal("load config failed: ", err)
		return
	}

	var ctx = context.Background()
	var defaultTimeout = 10 * 60 * time.Second
	ctx, cancel := context.WithTimeout(ctx, cfg.GetTimeout(defaultTimeout))
	defer cancel()

	err = runner.Prepare(ctx, cfg)
	if err != nil {
		log.Println("failed in prepare linter:", err)
		return
	}

	outputs, err := runner.Run(ctx, cfg)
	if err != nil {
		log.Println("failed in run linter:", err)
		return
	}
	if len(outputs) == 0 {
		log.Println("no valid output after run")
		return
	}

	outputs = runner.Parse(ctx, cfg, outputs)
	runner.PrintOutput(ctx, cfg, outputs)
	// create comment on issue
	if cfg.LinterCfg.Issue.Comment && cfg.LinterCfg.Issue.ID > 0 {
		err = runner.CreateIssueComment(ctx, cfg, outputs)
		if err != nil {
			log.Printf("failed to SaveOutputs err %+v \n", err)
			return
		}
	}
	if cfg.LinterCfg.ExitFail {
		os.Exit(1)
	}
}
