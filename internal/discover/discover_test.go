package discover

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const sampleRoot = "../../examples/sample"

func findRoot(t *testing.T, roots []RootModule, dir string) RootModule {
	t.Helper()
	for _, r := range roots {
		if r.Dir == dir {
			return r
		}
	}
	t.Fatalf("root module %q not found in %+v", dir, roots)
	return RootModule{}
}

func TestScanEnumeratesRootModules(t *testing.T) {
	roots, err := Scan(sampleRoot)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got := make([]string, len(roots))
	for i, r := range roots {
		got[i] = r.Dir
	}
	want := []string{"envs/dev", "envs/prod", "services/api", "standalone"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root modules = %v, want %v", got, want)
	}
	// Shared modules define neither a backend nor a provider, so they must not
	// be treated as roots.
	for _, r := range roots {
		if r.Dir == "modules/vpc" || r.Dir == "modules/subnet" || r.Dir == "modules/app" {
			t.Fatalf("shared module %q was misclassified as a root", r.Dir)
		}
	}
}

func TestTransitiveModuleDeps(t *testing.T) {
	roots, err := Scan(sampleRoot)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	api := findRoot(t, roots, "services/api")
	// api -> app -> vpc -> subnet must all be present transitively.
	want := []string{"modules/app", "modules/subnet", "modules/vpc"}
	if !reflect.DeepEqual(api.ModuleDeps, want) {
		t.Fatalf("services/api deps = %v, want %v", api.ModuleDeps, want)
	}

	dev := findRoot(t, roots, "envs/dev")
	wantDev := []string{"modules/subnet", "modules/vpc"}
	if !reflect.DeepEqual(dev.ModuleDeps, wantDev) {
		t.Fatalf("envs/dev deps = %v, want %v", dev.ModuleDeps, wantDev)
	}
}

func TestEnvironmentsFromTFVars(t *testing.T) {
	roots, err := Scan(sampleRoot)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	api := findRoot(t, roots, "services/api")
	want := []string{"production", "staging"}
	if !reflect.DeepEqual(api.Environments, want) {
		t.Fatalf("services/api environments = %v, want %v", api.Environments, want)
	}

	standalone := findRoot(t, roots, "standalone")
	if len(standalone.Environments) != 0 {
		t.Fatalf("standalone environments = %v, want none", standalone.Environments)
	}
	if !findRoot(t, roots, "envs/prod").HasBackend {
		t.Fatalf("envs/prod should report HasBackend")
	}
}

func TestIsLocalSource(t *testing.T) {
	cases := map[string]bool{
		"./mod":                     true,
		"../mod":                    true,
		"../../modules/vpc":         true,
		"terraform-aws-modules/vpc": false,
		"git::https://x/y.git":      false,
		"":                          false,
	}
	for src, want := range cases {
		if got := isLocalSource(src); got != want {
			t.Errorf("isLocalSource(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestExactVersion(t *testing.T) {
	cases := map[string]string{
		"1.9.8":           "1.9.8",
		"= 1.9.8":         "1.9.8",
		"=1.9.8":          "1.9.8",
		" v1.6.0-beta1 ":  "1.6.0-beta1",
		">= 1.5.0":        "",
		"~> 1.9.0":        "",
		"1.9":             "",
		"1.9.8, != 1.9.7": "",
		"latest":          "",
		"latest:^1.5":     "",
		"":                "",
	}
	for in, want := range cases {
		if got := exactVersion(in); got != want {
			t.Errorf("exactVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// writeFiles creates files (slash-separated paths relative to dir) with the
// given contents.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVersionPins(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		// Exact required_version, no version files.
		"pinned/main.tf": "terraform {\n  required_version = \"= 1.9.8\"\n}\nprovider \"aws\" {}\n",
		// Range constraint: nothing pinned from HCL; inherits the repo-level
		// .terraform-version.
		"ranged/main.tf": "terraform {\n  required_version = \">= 1.5.0\"\n}\nprovider \"aws\" {}\n",
		// Nearer files override the repo-level ones.
		"nested/app/main.tf":           "provider \"aws\" {}\n",
		"nested/.terraform-version":    "1.10.5\n",
		"nested/app/.opentofu-version": "1.8.3\n",
		// The nearest file is "latest": not exact, and it still shadows the
		// repo-level file.
		"floating/main.tf":            "provider \"aws\" {}\n",
		"floating/.terraform-version": "latest\n",
		".terraform-version":          "1.7.0\n",
	})
	roots, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	type pins struct{ req, tf, tofu string }
	want := map[string]pins{
		"pinned":     {req: "1.9.8", tf: "1.7.0"},
		"ranged":     {tf: "1.7.0"},
		"nested/app": {tf: "1.10.5", tofu: "1.8.3"},
		"floating":   {},
	}
	for d, w := range want {
		r := findRoot(t, roots, d)
		got := pins{r.RequiredVersion, r.TerraformVersionFile, r.OpenTofuVersionFile}
		if got != w {
			t.Errorf("%s pins = %+v, want %+v", d, got, w)
		}
	}
}
