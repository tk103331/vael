package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, "echo hello && echo world", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "hello") || !strings.Contains(result.Output, "world") {
		t.Fatalf("%q", result.Output)
	}
	if result.Code != 0 {
		t.Fatalf("exit code %d", result.Code)
	}
	if result.Duration < 0 {
		t.Fatalf("duration %d", result.Duration)
	}
}

func TestRunReportsAFailingCommand(t *testing.T) {
	result, err := Run(context.Background(), t.TempDir(), "exit 3", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != 3 {
		t.Fatalf("exit code %d", result.Code)
	}
}

func TestRunRunsInTheGivenDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), root, "ls", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "marker.txt") {
		t.Fatalf("%q", result.Output)
	}
}

func TestRunRejectsAnEmptyCommand(t *testing.T) {
	if _, err := Run(context.Background(), t.TempDir(), "  ", time.Second); err == nil {
		t.Fatal("empty command accepted")
	}
}

func TestRunStopsWhenItsTimeIsUp(t *testing.T) {
	start := time.Now()
	result, err := Run(context.Background(), t.TempDir(), "sleep 5", 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Fatalf("the command was not stopped: %+v", result)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the command ran past its time")
	}
}
