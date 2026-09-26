package main

import (
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"smalltalk/internal/app"
	"smalltalk/internal/chat"
	"smalltalk/internal/platform/config"
)

// TestOpenAPIMatchesRouter fails when a route is added, removed or renamed
// without updating docs/openapi.yaml, or the other way round.
func TestOpenAPIMatchesRouter(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	var documented []string
	for path, item := range spec.Paths {
		for method := range item {
			if method == "parameters" {
				continue
			}
			documented = append(documented, strings.ToUpper(method)+" "+path)
		}
	}

	// The router only needs a pool for real requests; building it is enough here.
	cfg := config.Config{JWTSecret: []byte(strings.Repeat("s", 32)), MediaDir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := app.NewRouter(ctx, cfg, nil, chat.NewMemoryBroker())
	if err != nil {
		t.Fatal(err)
	}
	param := regexp.MustCompile(`:(\w+)`)
	var served []string
	for _, rt := range r.Routes() {
		served = append(served, rt.Method+" "+param.ReplaceAllString(rt.Path, "{$1}"))
	}

	slices.Sort(documented)
	slices.Sort(served)
	for _, s := range served {
		if !slices.Contains(documented, s) {
			t.Errorf("route %s is served but missing from docs/openapi.yaml", s)
		}
	}
	for _, d := range documented {
		if !slices.Contains(served, d) {
			t.Errorf("docs/openapi.yaml documents %s but the router does not serve it", d)
		}
	}
}
