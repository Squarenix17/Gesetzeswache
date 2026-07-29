package config

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var envExampleLine = regexp.MustCompile(`^GEW_[A-Z0-9_]+=`)

// runtimeInternalEnv vars are read indirectly or are operator-only and need not appear in .env.example.
var runtimeInternalEnv = map[string]struct{}{}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func parseEnvExample(t *testing.T) map[string]struct{} {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".env.example")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := envExampleLine.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		key := strings.SplitN(line, "=", 2)[0]
		out[key] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func configConsumedEnv(t *testing.T) map[string]struct{} {
	t.Helper()
	cfgPath := filepath.Join(repoRoot(t), "internal", "config", "config.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, cfgPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	helpers := map[string]struct{}{
		"env": {}, "envDur": {}, "envFloat": {}, "envInt": {}, "envBool": {},
	}
	out := map[string]struct{}{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if _, isHelper := helpers[sel.Name]; !isHelper {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		key := strings.Trim(lit.Value, `"`)
		if strings.HasPrefix(key, "GEW_") {
			out[key] = struct{}{}
		}
		return true
	})
	// GEW_SHARED_SECRET uses os.Getenv directly.
	out["GEW_SHARED_SECRET"] = struct{}{}
	return out
}

func TestEnvExample_matchesConfig(t *testing.T) {
	example := parseEnvExample(t)
	consumed := configConsumedEnv(t)

	for key := range example {
		if _, skip := runtimeInternalEnv[key]; skip {
			continue
		}
		if _, ok := consumed[key]; !ok {
			t.Errorf(".env.example documents %s but config.Load does not consume it", key)
		}
	}
	for key := range consumed {
		if _, skip := runtimeInternalEnv[key]; skip {
			continue
		}
		if _, ok := example[key]; !ok {
			t.Errorf("config.Load consumes %s but it is missing from .env.example", key)
		}
	}
}
