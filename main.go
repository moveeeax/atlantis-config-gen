// Command atlantis-config-gen derives an atlantis.yaml from a repository's
// Terraform directory layout: it finds root modules, resolves the local modules
// they use (transitively), and emits Atlantis projects whose autoplan triggers
// cover every file that can affect a plan.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/moveeeax/atlantis-config-gen/internal/discover"
	"github.com/moveeeax/atlantis-config-gen/internal/generate"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "atlantis-config-gen:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	fs := flag.NewFlagSet("atlantis-config-gen", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root to scan")
	out := fs.String("out", "atlantis.yaml", `output file ("-" for stdout)`)
	diff := fs.Bool("diff", false, "dry-run: print a diff against the existing --out and exit 2 if it differs")
	workflow := fs.String("workflow", "", "workflow name to attach to every project")
	nameTmpl := fs.String("name-template", generate.DefaultNameTemplate,
		"Go template for project names (fields: .Dir .Base .Path .Env)")
	automerge := fs.Bool("automerge", false, "set automerge: true in the generated config")
	parallel := fs.Bool("parallel", false, "set parallel_plan and parallel_apply: true")
	showVer := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVer {
		fmt.Fprintln(stdout, version)
		return nil
	}

	roots, err := discover.Scan(*root)
	if err != nil {
		return err
	}
	cfg, err := generate.Build(roots, generate.Options{
		Workflow:     *workflow,
		NameTemplate: *nameTmpl,
		Automerge:    *automerge,
		Parallel:     *parallel,
	})
	if err != nil {
		return err
	}
	rendered, err := generate.Render(cfg)
	if err != nil {
		return err
	}

	if *diff {
		var existing []byte
		if *out != "-" {
			existing, _ = os.ReadFile(*out)
		}
		d := generate.UnifiedDiff(string(existing), string(rendered), *out)
		if d == "" {
			fmt.Fprintf(stderr, "%s is up to date (%d project(s))\n", *out, len(cfg.Projects))
			return nil
		}
		fmt.Fprint(stdout, d)
		os.Exit(2)
	}

	if *out == "-" {
		_, err = stdout.Write(rendered)
		return err
	}
	if err := os.WriteFile(*out, rendered, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "wrote %d project(s) to %s\n", len(cfg.Projects), *out)
	return nil
}
