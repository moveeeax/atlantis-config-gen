// Package discover walks a repository, identifies Terraform root modules
// (directories that configure a backend or a provider), and resolves the local
// child modules each root uses, transitively.
package discover

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// RootModule is a Terraform root module found under the scan root. All paths are
// slash-separated and relative to the scan root.
type RootModule struct {
	Dir          string   // e.g. "envs/prod" or "." for the root itself
	HasBackend   bool     // has a terraform { backend|cloud {} } block
	HasProvider  bool     // has at least one provider {} block
	HasTFVars    bool     // has any *.tfvars / *.tfvars.json file
	Environments []string // env-selecting tfvars stems (excludes auto/terraform tfvars), sorted
	ModuleDeps   []string // transitively-used local module dirs, sorted
}

// moduleInfo is the raw per-directory data gathered before roots are selected.
type moduleInfo struct {
	hasBackend  bool
	hasProvider bool
	hasTFVars   bool
	localCalls  []string // resolved dirs (relative to root) of local module sources
	environ     []string // env-selecting tfvars stems
}

// Scan walks root and returns every root module, sorted by Dir.
func Scan(root string) ([]RootModule, error) {
	root = filepath.Clean(root)
	infos := map[string]*moduleInfo{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && strings.HasPrefix(name, ".") {
			return filepath.SkipDir
		}
		if name == ".terraform" || name == "vendor" {
			return filepath.SkipDir
		}
		info, ok, perr := parseDir(root, path)
		if perr != nil {
			return perr
		}
		if ok {
			infos[relTo(root, path)] = info
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var roots []RootModule
	for rel, info := range infos {
		if !(info.hasBackend || info.hasProvider) {
			continue
		}
		roots = append(roots, RootModule{
			Dir:          rel,
			HasBackend:   info.hasBackend,
			HasProvider:  info.hasProvider,
			HasTFVars:    info.hasTFVars,
			Environments: info.environ,
			ModuleDeps:   transitiveDeps(rel, infos),
		})
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Dir < roots[j].Dir })
	return roots, nil
}

// parseDir inspects a single directory. It returns ok=false when the directory
// holds no .tf files (nothing to describe).
func parseDir(root, dir string) (*moduleInfo, bool, error) {
	tfFiles, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, false, err
	}
	if len(tfFiles) == 0 {
		return nil, false, nil
	}
	sort.Strings(tfFiles)

	info := &moduleInfo{}
	p := hclparse.NewParser()
	for _, f := range tfFiles {
		hf, diag := p.ParseHCLFile(f)
		if diag.HasErrors() {
			// A file that does not parse cannot contribute config; skip it
			// rather than failing the whole scan.
			continue
		}
		body, ok := hf.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, b := range body.Blocks {
			switch b.Type {
			case "provider":
				info.hasProvider = true
			case "terraform":
				for _, ib := range b.Body.Blocks {
					if ib.Type == "backend" || ib.Type == "cloud" {
						info.hasBackend = true
					}
				}
			case "module":
				src := stringAttr(b.Body, "source")
				if isLocalSource(src) {
					abs := filepath.Join(dir, src)
					info.localCalls = append(info.localCalls, relTo(root, abs))
				}
			}
		}
	}

	info.environ, info.hasTFVars = tfvarsEnvironments(dir)
	return info, true, nil
}

// tfvarsEnvironments returns the environment-selecting tfvars stems in dir (a
// file named prod.tfvars yields "prod"). terraform.tfvars and *.auto.tfvars are
// auto-loaded defaults, not environment selectors, so they are excluded from the
// list but still flip hasTFVars.
func tfvarsEnvironments(dir string) (envs []string, hasAny bool) {
	seen := map[string]bool{}
	for _, pat := range []string{"*.tfvars", "*.tfvars.json"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, m := range matches {
			hasAny = true
			name := filepath.Base(m)
			if name == "terraform.tfvars" || name == "terraform.tfvars.json" ||
				strings.HasSuffix(name, ".auto.tfvars") || strings.HasSuffix(name, ".auto.tfvars.json") {
				continue
			}
			stem := strings.TrimSuffix(strings.TrimSuffix(name, ".json"), ".tfvars")
			seen[stem] = true
		}
	}
	for s := range seen {
		envs = append(envs, s)
	}
	sort.Strings(envs)
	return envs, hasAny
}

// transitiveDeps returns the local module directories reachable from start via
// module calls, following child modules' own calls (so a change deep in the tree
// still triggers a plan on the root that pulls it in).
func transitiveDeps(start string, infos map[string]*moduleInfo) []string {
	seen := map[string]bool{}
	var queue []string
	if info, ok := infos[start]; ok {
		queue = append(queue, info.localCalls...)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if info, ok := infos[cur]; ok {
			queue = append(queue, info.localCalls...)
		}
	}
	deps := make([]string, 0, len(seen))
	for d := range seen {
		deps = append(deps, d)
	}
	sort.Strings(deps)
	return deps
}

// stringAttr reads a literal string attribute; anything non-literal yields "".
func stringAttr(body *hclsyntax.Body, name string) string {
	attr, ok := body.Attributes[name]
	if !ok {
		return ""
	}
	v, diag := attr.Expr.Value(nil)
	if diag.HasErrors() || v.IsNull() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

func isLocalSource(src string) bool {
	return strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../")
}

// relTo returns path relative to root, slash-separated; "." for root itself.
func relTo(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		rel = path
	}
	return filepath.ToSlash(rel)
}
