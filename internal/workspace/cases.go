package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sven97/lazyleet/internal/testcase"
)

var ErrCasesChanged = errors.New("testcases.jsonl changed outside the manager; reopen it before saving")

// CaseSnapshot couples parsed cases to their original bytes so edits can detect
// external changes and retain comments, blank lines, and untouched cases.
type CaseSnapshot struct {
	Cases  []testcase.Case
	raw    []byte
	exists bool
}

func (w *Workspace) ReadCaseSnapshot() (CaseSnapshot, error) {
	raw, err := os.ReadFile(w.TestsPath)
	if os.IsNotExist(err) {
		return CaseSnapshot{}, nil
	}
	if err != nil {
		return CaseSnapshot{}, err
	}
	cases, err := testcase.Parse(bytes.NewReader(raw))
	return CaseSnapshot{Cases: cases, raw: raw, exists: true}, err
}

// EditCase adds at index -1, replaces an existing case, or deletes it when c is
// nil. The original file remains intact if validation or writing fails.
func (w *Workspace) EditCase(snapshot CaseSnapshot, index int, c *testcase.Case) error {
	if index < -1 || index >= len(snapshot.Cases) || (index == -1 && c == nil) {
		return fmt.Errorf("invalid case index %d", index)
	}
	var encoded []byte
	if c != nil {
		if len(c.In) == 0 {
			return fmt.Errorf("a case needs at least one input")
		}
		var err error
		encoded, err = json.Marshal(c)
		if err != nil {
			return err
		}
		if len(encoded) >= 4*1024*1024 {
			return fmt.Errorf("case exceeds the 4 MiB file-format limit")
		}
		encoded = append(encoded, '\n')
	}
	var out bytes.Buffer
	if index == -1 {
		out.Write(snapshot.raw)
		if len(snapshot.raw) > 0 && snapshot.raw[len(snapshot.raw)-1] != '\n' {
			out.WriteByte('\n')
		}
		out.Write(encoded)
	} else {
		current := 0
		for _, line := range strings.SplitAfter(string(snapshot.raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				out.WriteString(line)
				continue
			}
			if current == index {
				out.Write(encoded)
			} else {
				out.WriteString(line)
			}
			current++
		}
	}
	return w.replaceCases(snapshot, out.Bytes())
}

func (w *Workspace) replaceCases(snapshot CaseSnapshot, data []byte) error {
	// Validate the entire candidate before touching the existing file.
	if _, err := testcase.Parse(bytes.NewReader(data)); err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(w.TestsPath); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(w.TestsPath), ".testcases-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.ReadFile(w.TestsPath)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if exists != snapshot.exists || !bytes.Equal(current, snapshot.raw) {
		return ErrCasesChanged
	}
	return os.Rename(name, w.TestsPath)
}
