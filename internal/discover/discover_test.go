package discover

import (
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
