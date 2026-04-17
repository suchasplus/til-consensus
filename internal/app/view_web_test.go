package app

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestViewCommandWebStartsServerAndPrintsURL(t *testing.T) {
	resultPath, err := filepath.Abs(filepath.Join("..", "..", "testdata", "view", "sample-run", "result.json"))
	if err != nil {
		t.Fatalf("resolve result path: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := newViewCommand()
	stdout := &syncBuffer{}
	stderr := &syncBuffer{}
	cmd.Writer = stdout
	cmd.ErrWriter = stderr

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- cmd.Run(ctx, []string{
			"view",
			"--web",
			"--result", resultPath,
			"--host", "127.0.0.1",
			"--port", "0",
			"--format", "json",
		})
	}()

	urlValue, err := waitForWebURL(stdout, doneCh)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(urlValue, "http://127.0.0.1:") {
		t.Fatalf("unexpected web url: %s", urlValue)
	}

	healthzURL := urlValue + "/api/healthz"
	if err := waitForHealthz(healthzURL, doneCh); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "view --web 忽略 --format=json") {
		t.Fatalf("expected format ignore warning, got stderr:\n%s", stderr.String())
	}

	cancel()
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("view command returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for web viewer to stop")
	}
}

func waitForWebURL(stdout *syncBuffer, doneCh <-chan error) (string, error) {
	deadline := time.Now().Add(5 * time.Second)
	re := regexp.MustCompile(`web viewer started: (http://[^\s]+)`)
	for time.Now().Before(deadline) {
		if exited, err := tryReceiveCommandError(doneCh); exited {
			if err != nil {
				return "", fmt.Errorf("view command exited before startup: %w", err)
			}
			return "", fmt.Errorf("view command exited before startup")
		}
		if match := re.FindStringSubmatch(stdout.String()); len(match) == 2 {
			return match[1], nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return "", fmt.Errorf("timed out waiting for web viewer URL, stdout:\n%s", stdout.String())
}

func waitForHealthz(urlValue string, doneCh <-chan error) error {
	client := &http.Client{Timeout: 400 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if exited, err := tryReceiveCommandError(doneCh); exited {
			if err != nil {
				return fmt.Errorf("view command exited before healthz: %w", err)
			}
			return fmt.Errorf("view command exited before healthz")
		}
		resp, err := client.Get(urlValue)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for healthz: %s", urlValue)
}

func tryReceiveCommandError(doneCh <-chan error) (bool, error) {
	select {
	case err := <-doneCh:
		return true, err
	default:
		return false, nil
	}
}
