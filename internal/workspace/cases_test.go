package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sven97/lazyleet/internal/testcase"
)

func TestCaseEditsPreserveCommentsAndDetectExternalChanges(t *testing.T) {
	w := &Workspace{TestsPath: filepath.Join(t.TempDir(), "testcases.jsonl")}
	original := "# my note\n\n{\"in\":[\"1\"],\"out\":\"2\"}\n# keep this too\n{\"in\":[\"3\"]}"
	if err := os.WriteFile(w.TestsPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := w.ReadCaseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = w.EditCase(snap, 0, &testcase.Case{In: []string{"4"}, Out: "5"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(w.TestsPath)
	if !strings.Contains(string(data), "# my note\n\n") || !strings.Contains(string(data), "# keep this too\n{\"in\":[\"3\"]}") {
		t.Fatalf("lost comments or untouched case: %s", data)
	}
	info, _ := os.Stat(w.TestsPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions changed")
	}
	if err = w.EditCase(snap, 1, nil); !errors.Is(err, ErrCasesChanged) {
		t.Fatalf("stale snapshot overwrote external change: %v", err)
	}
	unchanged, _ := os.ReadFile(w.TestsPath)
	if string(unchanged) != string(data) {
		t.Fatal("conflict changed file")
	}
	snap, err = w.ReadCaseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = w.EditCase(snap, 1, nil); err != nil {
		t.Fatal(err)
	}
	snap, err = w.ReadCaseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Cases) != 1 {
		t.Fatal("delete failed")
	}
	if err = w.EditCase(snap, -1, &testcase.Case{In: []string{"6"}}); err != nil {
		t.Fatal(err)
	}
	got, err := w.ReadCases()
	if err != nil || len(got) != 2 || got[1].In[0] != "6" {
		t.Fatalf("add failed: %+v %v", got, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(w.TestsPath), ".testcases-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary files leaked")
	}
}

func TestInvalidCaseAndExternalDeletionLeaveOriginal(t *testing.T) {
	dir := t.TempDir()
	w := &Workspace{TestsPath: filepath.Join(dir, "testcases.jsonl")}
	if err := os.WriteFile(w.TestsPath, []byte("{\"in\":[\"1\"]}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	snap, err := w.ReadCaseSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = w.EditCase(snap, 0, &testcase.Case{}); err == nil {
		t.Fatal("accepted empty inputs")
	}
	// An external deletion is a conflict, not permission to recreate the file.
	if err = os.Remove(w.TestsPath); err != nil {
		t.Fatal(err)
	}
	if err = w.EditCase(snap, 0, &testcase.Case{In: []string{"2"}}); !errors.Is(err, ErrCasesChanged) {
		t.Fatal(err)
	}
	if _, err = os.Stat(w.TestsPath); !os.IsNotExist(err) {
		t.Fatal("recreated externally deleted file")
	}
}

func TestAppendCasesIsSafeAndPreservesAnnotations(t *testing.T) {
	w := &Workspace{TestsPath: filepath.Join(t.TempDir(), "testcases.jsonl")}
	if err := os.WriteFile(w.TestsPath, []byte("# annotated\n{\"in\":[\"1\"]}"), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := w.AppendCases([]testcase.Case{{In: []string{"1"}}, {In: []string{"2"}, Out: "3"}})
	if err != nil || n != 1 {
		t.Fatalf("append %d: %v", n, err)
	}
	data, _ := os.ReadFile(w.TestsPath)
	if !strings.HasPrefix(string(data), "# annotated\n") {
		t.Fatal("lost annotation")
	}
	got, err := w.ReadCases()
	if err != nil || len(got) != 2 {
		t.Fatalf("invalid append: %+v %v", got, err)
	}
	before := string(data)
	if _, err = w.AppendCases([]testcase.Case{{}}); err == nil {
		t.Fatal("invalid append succeeded")
	}
	after, _ := os.ReadFile(w.TestsPath)
	if string(after) != before {
		t.Fatal("failed append damaged file")
	}
}
