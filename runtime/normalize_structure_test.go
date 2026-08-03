package runtime

import (
	"bufio"
	"os"
	"testing"
)

func TestTaskContractSourceFilesStayFocused(t *testing.T) {
	limits := map[string]int{
		"normalize.go":     500,
		"task_decode.go":   500,
		"task_schema.go":   500,
		"task_validate.go": 500,
	}
	for path, limit := range limits {
		assertTaskFileLineLimit(t, path, limit)
	}
}

func assertTaskFileLineLimit(t *testing.T, path string, limit int) {
	t.Helper()
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
