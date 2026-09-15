package runner

import (
	"fmt"
	"strings"
)

// normalizeLCType canonicalizes LeetCode type strings.
func normalizeLCType(t string) string {
	t = strings.TrimSpace(strings.ToLower(t))
	t = strings.ReplaceAll(t, " ", "")
	return t
}

func lcToGoType(t string) (string, error) {
	t = normalizeLCType(t)
	switch {
	case t == "integer" || t == "int":
		return "int", nil
	case t == "long" || t == "longinteger":
		return "int64", nil
	case t == "boolean" || t == "bool":
		return "bool", nil
	case t == "string":
		return "string", nil
	case t == "double" || t == "float" || t == "float64":
		return "float64", nil
	case t == "character" || t == "char":
		return "byte", nil
	case strings.HasPrefix(t, "list<") && strings.HasSuffix(t, ">"):
		inner, err := lcToGoType(t[5 : len(t)-1])
		if err != nil {
			return "", err
		}
		return "[]" + inner, nil
	case strings.HasSuffix(t, "[]"):
		inner, err := lcToGoType(strings.TrimSuffix(t, "[]"))
		if err != nil {
			return "", err
		}
		return "[]" + inner, nil
	default:
		return "", fmt.Errorf("unsupported Go type mapping for %q", t)
	}
}

func lcToJavaType(t string) (string, error) {
	t = normalizeLCType(t)
	switch {
	case t == "integer" || t == "int":
		return "int", nil
	case t == "long" || t == "longinteger":
		return "long", nil
	case t == "boolean" || t == "bool":
		return "boolean", nil
	case t == "string":
		return "String", nil
	case t == "double" || t == "float":
		return "double", nil
	case t == "character" || t == "char":
		return "char", nil
	case strings.HasPrefix(t, "list<") && strings.HasSuffix(t, ">"):
		inner := t[5 : len(t)-1]
		boxed, err := lcToJavaBoxed(inner)
		if err != nil {
			return "", err
		}
		return "java.util.List<" + boxed + ">", nil
	case strings.HasSuffix(t, "[]"):
		inner, err := lcToJavaType(strings.TrimSuffix(t, "[]"))
		if err != nil {
			return "", err
		}
		return inner + "[]", nil
	default:
		return "", fmt.Errorf("unsupported Java type mapping for %q", t)
	}
}

func lcToJavaBoxed(t string) (string, error) {
	t = normalizeLCType(t)
	if strings.HasPrefix(t, "list<") {
		return lcToJavaType(t)
	}
	if strings.HasSuffix(t, "[]") {
		return lcToJavaType(t)
	}
	switch t {
	case "integer", "int":
		return "Integer", nil
	case "long", "longinteger":
		return "Long", nil
	case "boolean", "bool":
		return "Boolean", nil
	case "string":
		return "String", nil
	case "double", "float":
		return "Double", nil
	case "character", "char":
		return "Character", nil
	default:
		return "", fmt.Errorf("unsupported Java boxed type for %q", t)
	}
}

func lcToCppType(t string) (string, error) {
	t = normalizeLCType(t)
	switch {
	case t == "integer" || t == "int":
		return "int", nil
	case t == "long" || t == "longinteger":
		return "long long", nil
	case t == "boolean" || t == "bool":
		return "bool", nil
	case t == "string":
		return "string", nil
	case t == "double" || t == "float":
		return "double", nil
	case t == "character" || t == "char":
		return "char", nil
	case strings.HasPrefix(t, "list<") && strings.HasSuffix(t, ">"):
		inner, err := lcToCppType(t[5 : len(t)-1])
		if err != nil {
			return "", err
		}
		return "vector<" + inner + ">", nil
	case strings.HasSuffix(t, "[]"):
		inner, err := lcToCppType(strings.TrimSuffix(t, "[]"))
		if err != nil {
			return "", err
		}
		return "vector<" + inner + ">", nil
	default:
		return "", fmt.Errorf("unsupported C++ type mapping for %q", t)
	}
}
