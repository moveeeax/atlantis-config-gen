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

func TestTerraformVersionAndDistribution(t *testing.T) {
	roots := []discover.RootModule{
		{Dir: "a", HasProvider: true, RequiredVersion: "1.9.8"},
		{Dir: "b", HasProvider: true, TerraformVersionFile: "1.10.5", OpenTofuVersionFile: "1.8.3"},
		{Dir: "c", HasProvider: true, TerraformVersionFile: "1.10.5"},
		{Dir: "d", HasProvider: true},
		// An exact required_version beats a disagreeing version file.
		{Dir: "e", HasProvider: true, RequiredVersion: "1.9.8", TerraformVersionFile: "1.10.5", OpenTofuVersionFile: "1.8.3"},
	}
	cases := []struct {
		dist string
		want map[string]string // dir -> terraform_version
	}{
		{"", map[string]string{"a": "1.9.8", "b": "1.10.5", "c": "1.10.5", "d": "", "e": "1.9.8"}},
		{DistributionTerraform, map[string]string{"a": "1.9.8", "b": "1.10.5", "c": "1.10.5", "d": "", "e": "1.9.8"}},
		// opentofu reads .opentofu-version and ignores .terraform-version.
		{DistributionOpenTofu, map[string]string{"a": "1.9.8", "b": "1.8.3", "c": "", "d": "", "e": "1.9.8"}},
	}
	for _, tc := range cases {
		cfg, err := Build(roots, Options{Distribution: tc.dist})
		if err != nil {
			t.Fatalf("Build(%q): %v", tc.dist, err)
		}
		for _, p := range cfg.Projects {
			if p.TerraformVersion != tc.want[p.Dir] {
				t.Errorf("dist %q: %s terraform_version = %q, want %q", tc.dist, p.Dir, p.TerraformVersion, tc.want[p.Dir])
			}
			if p.TerraformDist != tc.dist {
				t.Errorf("dist %q: %s terraform_distribution = %q", tc.dist, p.Dir, p.TerraformDist)
			}
		}
	}

	cfg, _ := Build(roots[1:2], Options{Distribution: DistributionOpenTofu})
	out, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"terraform_distribution: opentofu\n", "terraform_version: 1.8.3\n"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("rendered config missing %q:\n%s", want, out)
		}
	}
	cfg, _ = Build(roots[3:4], Options{})
	out, _ = Render(cfg)
	if strings.Contains(string(out), "terraform_") {
		t.Errorf("unpinned project should omit terraform_* keys:\n%s", out)
	}
}

func TestUnknownDistributionRejected(t *testing.T) {
	if _, err := Build(sampleRoots(), Options{Distribution: "tofu"}); err == nil {
		t.Fatal("expected an error for an unknown distribution")
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
