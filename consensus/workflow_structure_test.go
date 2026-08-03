package consensus

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowSourceFilesStayFocused(t *testing.T) {
	assertGoFileLineLimit(t, "workflow*.go", 1300)
}

func assertGoFileLineLimit(t *testing.T, pattern string, limit int) {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		lines := 0
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines++
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if lines > limit {
			t.Errorf("%s has %d lines; split it before it exceeds %d lines", path, lines, limit)
		}
	}
}
