package review

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/borch-ai/powerword/internal/loop"
	"github.com/borch-ai/powerword/pkg/config"
)

var loopRunLoop = loop.RunLoop

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := execCommand(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func handleInterrupt(cancel context.CancelFunc, gitRollback bool) chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		if gitRollback {
			fmt.Println("\nReceived interrupt. Aborting autonomous loop...")
		} else {
			fmt.Println("\nReceived interrupt. Aborting autonomous loop and restoring workspace...")
			_ = execCommand(context.Background(), "git", "stash").Run()
			_ = execCommand(context.Background(), "git", "reset", "--hard", "HEAD").Run()
		}
		cancel()
	}()
	return sigChan
}

func buildPrompt(plan *Plan, diffStr string, makeOut []byte, makeErr error) string {
	var pb strings.Builder
	fmt.Fprintf(&pb, "Implementation Plan Goal:\n%s\n\n", plan.Goal)
	fmt.Fprintf(&pb, "Implementation Plan Proposed Changes:\n%s\n\n", plan.Changes)
	fmt.Fprintf(&pb, "Implementation Plan Verification:\n%s\n\n", plan.Verification)

	diffStr = strings.TrimSpace(diffStr)
	if len(diffStr) > 0 {
		fmt.Fprintf(&pb, "Current Git Diff:\n%s\n\n", diffStr)
	} else {
		pb.WriteString("Current Git Diff:\nNo changes yet.\n\n")
	}

	if makeErr != nil {
		fmt.Fprintf(&pb, "Local Validation Failed:\n```\n%s\n```\n", string(makeOut))
		pb.WriteString("Please fix the compilation, test, or lint errors above.\n")
	} else {
		pb.WriteString("Local Validation Passed.\n")
		pb.WriteString("If you believe the task is fully completed and all verification steps are satisfied, please reply exactly with 'TASK COMPLETE' and do nothing else.\n")
		pb.WriteString("Otherwise, continue implementing the changes.\n")
	}
	return pb.String()
}

func processTurnCompletion(ctx context.Context, cfg *config.Config, autoCfg *config.Config, makeErr error) (bool, error) {
	session, loadErr := loop.LoadSession(autoCfg.Session)
	if loadErr != nil || len(session.Messages) == 0 {
		return false, nil
	}

	lastMsg := session.Messages[len(session.Messages)-1].Content
	if strings.Contains(strings.ToUpper(lastMsg), "TASK COMPLETE") && makeErr == nil {
		fmt.Println("Agent reported task complete!")

		_ = runCommand(ctx, "git", "add", ".")
		//nolint:gosec // safe static commands with intended issue ID formatting
		_ = runCommand(ctx, "git", "commit", "-m", fmt.Sprintf("chore: autonomous repair for issue %s", cfg.Issue))
		_ = runCommand(ctx, "git", "push")

		//nolint:gosec // local CLI
		_ = runCommand(ctx, "gh", "issue", "close", cfg.Issue)
		//nolint:gosec // local CLI
		_ = runCommand(ctx, "gh", "issue", "comment", cfg.Issue, "--body", "Task complete. Changes have been pushed.")
		return true, nil
	}

	return false, nil
}

// RunAutonomousLoop orchestrates a 5-iteration autonomous loop to fix issues.
//
//nolint:gocognit,nestif,funlen
func RunAutonomousLoop(ctx context.Context, cfg *config.Config) (retErr error) {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigChan := handleInterrupt(cancel, cfg.GitRollback)
	defer signal.Stop(sigChan)

	var snapshot *loop.WorkspaceSnapshot
	if cfg.GitRollback {
		var snapErr error
		snapshot, snapErr = loop.NewWorkspaceSnapshot(ctx, "")
		if snapErr != nil {
			return fmt.Errorf("failed to initialize workspace rollback snapshot: %w", snapErr)
		}
		defer func() {
			if retErr != nil {
				fmt.Printf("Autonomous repair loop failed: %v. Rolling back workspace...\n", retErr)
				restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer restoreCancel()
				if restoreErr := snapshot.Restore(restoreCtx); restoreErr != nil {
					fmt.Printf("Warning: failed to restore workspace rollback snapshot: %v\n", restoreErr)
				}
			} else {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cleanupCancel()
				if cleanErr := snapshot.CleanUp(cleanupCtx); cleanErr != nil {
					fmt.Printf("Warning: failed to clean up workspace snapshot stash: %v\n", cleanErr)
				}
			}
		}()
	}

	if cfg.Issue == "" {
		return fmt.Errorf("--issue flag is required for autonomous mode")
	}

	fmt.Printf("Loading issue %s...\n", cfg.Issue)
	plan, err := LoadIssuePlan(loopCtx, cfg.Issue)
	if err != nil {
		return err
	}

	autoCfg := *cfg
	autoCfg.Headless = true
	autoCfg.AutoConfirm = true
	autoCfg.Session = fmt.Sprintf("auto-repair-%s", cfg.Issue)

	maxIterations := 5
	for i := 1; i <= maxIterations; i++ {
		if loopCtx.Err() != nil {
			return loopCtx.Err()
		}
		fmt.Printf("\n--- Autonomous Iteration %d/%d ---\n", i, maxIterations)

		fmt.Println("Running local validation (make all)...")
		makeOut, makeErr := execCommand(loopCtx, "make", "all").CombinedOutput()

		diffStr, extErr := ExtractGitDiff(loopCtx, &autoCfg)
		if extErr != nil {
			return fmt.Errorf("failed to extract git diff: %w", extErr)
		}

		prompt := buildPrompt(plan, diffStr, makeOut, makeErr)

		commentBody := fmt.Sprintf("Starting autonomous iteration %d...\n\nLocal Validation Status: %v", i, makeErr == nil)
		//nolint:gosec // local CLI intended with dynamic issue IDs
		_ = runCommand(loopCtx, "gh", "issue", "comment", cfg.Issue, "--body", commentBody)

		fmt.Println("Handing over to agent...")
		if runErr := loopRunLoop(loopCtx, &autoCfg, prompt); runErr != nil {
			fmt.Printf("Agent loop returned error: %v\n", runErr)
		}

		completed, cErr := processTurnCompletion(loopCtx, cfg, &autoCfg, makeErr)
		if cErr != nil {
			return cErr
		}
		if completed {
			return nil
		}

		_ = runCommand(loopCtx, "git", "add", ".")
		//nolint:gosec // safe usage
		_ = runCommand(loopCtx, "git", "commit", "-m", fmt.Sprintf("chore: autonomous repair turn %d", i))
		_ = runCommand(loopCtx, "git", "push")
	}

	return fmt.Errorf("reached maximum autonomous iterations (%d)", maxIterations)
}
