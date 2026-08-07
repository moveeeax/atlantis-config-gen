package generate

import (
	"strings"
	"testing"

	"github.com/moveeeax/atlantis-config-gen/internal/discover"
)

func sampleRoots() []discover.RootModule {
	return []discover.RootModule{
		{
			Dir: "services/api", HasBackend: true, HasProvider: true, HasTFVars: true,
			Environments: []string{"production", "staging"},
			ModuleDeps:   []string{"modules/app", "modules/subnet", "modules/vpc"},
		},
		{
			Dir: "standalone", HasBackend: true, HasProvider: true,
		},
	}
}

func TestBuildProjectsAndWorkspaces(t *testing.T) {
	cfg, err := Build(sampleRoots(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.Version != 3 {
		t.Fatalf("version = %d, want 3", cfg.Version)
	}
	// services/api yields two projects (one per env) + standalone default = 3.
	if len(cfg.Projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(cfg.Projects))
	}

	byName := map[string]Project{}
	for _, p := range cfg.Projects {
		byName[p.Name] = p
	}
	staging, ok := byName["api-staging"]
	if !ok {
		t.Fatalf("expected project api-staging, got %v", keys(byName))
	}
	if staging.Workspace != "staging" {
		t.Fatalf("api-staging workspace = %q, want staging", staging.Workspace)
	}
	if _, ok := byName["standalone"]; !ok {
		t.Fatalf("expected project standalone, got %v", keys(byName))
	}
}

func TestWhenModifiedIncludesTransitiveModules(t *testing.T) {
	cfg, err := Build(sampleRoots(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var api Project
	for _, p := range cfg.Projects {
		if p.Name == "api-staging" {
			api = p
		}
	}
	joined := strings.Join(api.Autoplan.WhenModified, "\n")
	for _, want := range []string{
		"*.tf",
		"*.tfvars",
		"../../modules/app/*.tf",
		"../../modules/vpc/*.tf",
		"../../modules/subnet/*.tf",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("when_modified missing %q; got:\n%s", want, joined)
		}
	}
	if !api.Autoplan.Enabled {
		t.Error("autoplan should be enabled")
	}
}

func TestRenderIsIdempotent(t *testing.T) {
	roots := sampleRoots()
	cfg1, _ := Build(roots, Options{})
	cfg2, _ := Build(roots, Options{})
	out1, err := Render(cfg1)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out2, _ := Render(cfg2)
	if string(out1) != string(out2) {
		t.Fatalf("render not stable across runs:\n%s\n---\n%s", out1, out2)
	}
	if d := UnifiedDiff(string(out1), string(out2), "atlantis.yaml"); d != "" {
		t.Fatalf("expected no diff, got:\n%s", d)
	}
}

func TestCustomNameTemplate(t *testing.T) {
	cfg, err := Build(sampleRoots(), Options{NameTemplate: "{{.Path}}{{if .Env}}-{{.Env}}{{end}}"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	found := false
	for _, p := range cfg.Projects {
		if p.Name == "services-api-production" {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom name template not applied; projects: %v", projectNames(cfg))
	}
}

func TestUnifiedDiffDetectsChange(t *testing.T) {
	if UnifiedDiff("a\n", "a\n", "x") != "" {
		t.Error("identical inputs should produce no diff")
	}
	if UnifiedDiff("a\n", "b\n", "x") == "" {
		t.Error("differing inputs should produce a diff")
	}
}

func keys(m map[string]Project) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func projectNames(cfg *Config) []string {
	out := make([]string, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		out = append(out, p.Name)
	}
	return out
}
