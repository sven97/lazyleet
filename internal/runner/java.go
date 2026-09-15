package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sven97/lazyleet/internal/leetcode"
	"github.com/sven97/lazyleet/internal/testcase"
)

type javaRunner struct{}

func (javaRunner) Available() bool {
	_, jerr := exec.LookPath("javac")
	_, rerr := exec.LookPath("java")
	return jerr == nil && rerr == nil
}

func (j javaRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if res, err, done := validateSpec(spec, "javac", j.Available()); done {
		return res, err
	}
	src, err := os.ReadFile(spec.SolutionPath)
	if err != nil {
		return Result{}, fmt.Errorf("runner: read solution: %w", err)
	}
	harness, err := buildJavaHarness(spec.Meta, spec.Cases)
	if err != nil {
		return Result{BuildErr: err.Error()}, nil
	}

	return runCompiled(ctx, spec, "lazyleet-java-*",
		func(dir string) error {
			if err := os.WriteFile(filepath.Join(dir, "Solution.java"), src, 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "Harness.java"), []byte(harness), 0o644)
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "javac", "Solution.java", "Harness.java")
			cmd.Dir = dir
			return cmd
		},
		func(ctx context.Context, dir string) *exec.Cmd {
			return exec.CommandContext(ctx, "java", "-cp", dir, "Harness")
		},
	)
}

func buildJavaHarness(meta leetcode.Meta, cases []testcase.Case) (string, error) {
	retType, err := lcToJavaType(meta.Return.Type)
	if err != nil {
		return "", err
	}
	paramTypes := make([]string, len(meta.Params))
	for i, p := range meta.Params {
		paramTypes[i], err = lcToJavaType(p.Type)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString("import java.util.*;\n")
	b.WriteString("import java.lang.reflect.*;\n")
	b.WriteString("public class Harness {\n")
	b.WriteString("  public static void main(String[] args) throws Exception {\n")
	b.WriteString("    StringBuilder sb = new StringBuilder();\n")
	b.WriteString("    sb.append(\"{\\\"build_err\\\":\\\"\\\",\\\"cases\\\":[\");\n")
	b.WriteString("    Solution sol = new Solution();\n")
	b.WriteString("    boolean first = true;\n")

	for i, c := range cases {
		b.WriteString("    {\n")
		b.WriteString("      if (!first) sb.append(\",\"); first = false;\n")
		b.WriteString("      long t0 = System.nanoTime();\n")
		b.WriteString(fmt.Sprintf("      int index = %d;\n", i))
		b.WriteString("      String status = \"ran\"; String actual = \"\"; String err = \"\";\n")
		b.WriteString("      java.io.PrintStream origOut = System.out;\n")
		b.WriteString("      java.io.ByteArrayOutputStream capBuf = new java.io.ByteArrayOutputStream();\n")
		b.WriteString("      System.setOut(new java.io.PrintStream(capBuf, true));\n")
		b.WriteString("      try {\n")
		args := make([]string, len(meta.Params))
		for pi := range meta.Params {
			if pi >= len(c.In) {
				return "", fmt.Errorf("case %d: missing arg %d", i, pi)
			}
			expr, err := javaLiteral(paramTypes[pi], c.In[pi])
			if err != nil {
				return "", err
			}
			b.WriteString(fmt.Sprintf("        %s arg%d = %s;\n", paramTypes[pi], pi, expr))
			args[pi] = fmt.Sprintf("arg%d", pi)
		}
		b.WriteString(fmt.Sprintf("        %s got = sol.%s(%s);\n", retType, meta.Name, strings.Join(args, ", ")))
		b.WriteString("        actual = toJson(got);\n")
		b.WriteString("      } catch (Throwable e) {\n")
		b.WriteString("        status = \"error\"; err = e.toString();\n")
		b.WriteString("      } finally {\n")
		b.WriteString("        System.setOut(origOut);\n")
		b.WriteString("      }\n")
		b.WriteString("      String stdout = capBuf.toString();\n")
		b.WriteString("      double ms = (System.nanoTime() - t0) / 1e6;\n")
		b.WriteString("      sb.append(\"{\\\"index\\\":\" + index")
		b.WriteString(" + \",\\\"status\\\":\\\"\" + status + \"\\\"\"")
		b.WriteString(" + \",\\\"actual\\\":\" + jsonString(actual)")
		b.WriteString(" + \",\\\"stdout\\\":\" + jsonString(stdout)")
		b.WriteString(" + \",\\\"err\\\":\" + jsonString(err)")
		b.WriteString(" + \",\\\"elapsed_ms\\\":\" + ms + \"}\");\n")
		b.WriteString("    }\n")
	}
	b.WriteString("    sb.append(\"]}\");\n")
	b.WriteString("    System.out.print(sb.toString());\n")
	b.WriteString("  }\n")
	b.WriteString(javaJSONHelpers)
	b.WriteString("}\n")
	_ = retType
	return b.String(), nil
}

func javaLiteral(javaType, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	jt := strings.ReplaceAll(javaType, " ", "")
	switch {
	case jt == "char":
		return javaCharLiteral(raw)
	case jt == "int" || jt == "long" || jt == "double" || jt == "boolean":
		if jt == "long" && !strings.HasSuffix(strings.ToUpper(raw), "L") {
			return raw + "L", nil
		}
		return raw, nil
	case jt == "String":
		// raw is already a JSON string literal like "\"abc\"" or unquoted
		if strings.HasPrefix(raw, "\"") {
			return raw, nil
		}
		b, _ := json.Marshal(raw)
		return string(b), nil
	case strings.HasSuffix(jt, "[]"):
		return javaArrayLiteral(jt, raw)
	case strings.HasPrefix(jt, "java.util.List<"):
		return javaListLiteral(jt, raw)
	default:
		return "", fmt.Errorf("cannot build Java literal for type %s", javaType)
	}
}

// javaCharLiteral turns the JSON string value LeetCode uses for a `character`
// test-case argument (e.g. `"a"`) into a Java char literal (e.g. 'a').
func javaCharLiteral(raw string) (string, error) {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("parse char: %w", err)
	}
	rs := []rune(s)
	if len(rs) != 1 {
		return "", fmt.Errorf("expected single-character string for char literal, got %q", s)
	}
	var esc string
	switch rs[0] {
	case '\\':
		esc = `\\`
	case '\'':
		esc = `\'`
	case '\n':
		esc = `\n`
	case '\r':
		esc = `\r`
	case '\t':
		esc = `\t`
	default:
		esc = string(rs[0])
	}
	return "'" + esc + "'", nil
}

func javaArrayLiteral(javaType, raw string) (string, error) {
	elem := strings.TrimSuffix(javaType, "[]")
	var vals []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &vals); err != nil {
		return "", fmt.Errorf("parse array: %w", err)
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		expr, err := javaLiteral(elem, string(v))
		if err != nil {
			return "", err
		}
		parts[i] = expr
	}
	return fmt.Sprintf("new %s{%s}", javaType, strings.Join(parts, ",")), nil
}

func javaListLiteral(javaType, raw string) (string, error) {
	// java.util.List<Integer> or nested
	inner := javaType[len("java.util.List<") : len(javaType)-1]
	var vals []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &vals); err != nil {
		return "", err
	}
	parts := make([]string, len(vals))
	elemType := inner
	// List<Integer> elements use boxed literals; arrays use array types
	if inner == "Integer" {
		elemType = "int" // autobox
	} else if inner == "Long" {
		elemType = "long"
	} else if inner == "Double" {
		elemType = "double"
	} else if inner == "Boolean" {
		elemType = "boolean"
	} else if inner == "Character" {
		elemType = "char"
	} else if inner == "String" {
		elemType = "String"
	} else {
		elemType = inner // nested List
	}
	for i, v := range vals {
		expr, err := javaLiteral(elemType, string(v))
		if err != nil {
			return "", err
		}
		parts[i] = expr
	}
	return fmt.Sprintf("java.util.Arrays.asList(%s)", strings.Join(parts, ",")), nil
}

const javaJSONHelpers = `
  static String jsonString(String s) {
    if (s == null) return "\"\"";
    StringBuilder b = new StringBuilder("\"");
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      switch (c) {
        case '\\': b.append("\\\\"); break;
        case '"': b.append("\\\""); break;
        case '\n': b.append("\\n"); break;
        case '\r': b.append("\\r"); break;
        case '\t': b.append("\\t"); break;
        default: b.append(c);
      }
    }
    b.append('"');
    return b.toString();
  }
  static String toJson(Object o) {
    if (o == null) return "null";
    if (o instanceof String) {
      return jsonString((String)o);
    }
    if (o instanceof Number || o instanceof Boolean) return o.toString();
    if (o instanceof int[]) {
      int[] a = (int[])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(a[i]); }
      return b.append("]").toString();
    }
    if (o instanceof long[]) {
      long[] a = (long[])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(a[i]); }
      return b.append("]").toString();
    }
    if (o instanceof double[]) {
      double[] a = (double[])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(a[i]); }
      return b.append("]").toString();
    }
    if (o instanceof boolean[]) {
      boolean[] a = (boolean[])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(a[i]); }
      return b.append("]").toString();
    }
    if (o instanceof Object[]) {
      Object[] a = (Object[])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(toJson(a[i])); }
      return b.append("]").toString();
    }
    if (o instanceof java.util.List) {
      java.util.List<?> a = (java.util.List<?>)o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.size(); i++) { if (i>0) b.append(","); b.append(toJson(a.get(i))); }
      return b.append("]").toString();
    }
    if (o instanceof int[][]) {
      int[][] a = (int[][])o;
      StringBuilder b = new StringBuilder("[");
      for (int i = 0; i < a.length; i++) { if (i>0) b.append(","); b.append(toJson(a[i])); }
      return b.append("]").toString();
    }
    return jsonString(String.valueOf(o));
  }
`
