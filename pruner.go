package tfdsprune

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

type Pruner struct {
	Dir string
	Out io.Writer

	files map[string]*hclwrite.File
}

func NewPruner(dir string) *Pruner {
	return &Pruner{
		Dir:   dir,
		Out:   os.Stdout,
		files: map[string]*hclwrite.File{},
	}
}

func (p *Pruner) Prune(inPlace bool) error {
	if err := p.load(); err != nil {
		return err
	}
	used := p.reachableDataKeys()
	changedFiles := map[string]bool{}
	p.removeUnusedDataBlocks(used, changedFiles)
	return p.writeOut(inPlace, changedFiles)
}

func (p *Pruner) load() error {
	pattern := filepath.Join(p.Dir, "*.tf")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob: %w", err)
	}
	sort.Strings(matches)
	var diags hcl.Diagnostics
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		f, parseDiags := hclwrite.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
		if parseDiags.HasErrors() {
			diags = append(diags, parseDiags...)
			continue
		}
		p.files[path] = f
	}
	if diags.HasErrors() {
		return diags
	}
	return nil
}

// reachableDataKeys returns the set of `<type>.<name>` keys for data sources
// that are reachable from non-`data` configuration. Refs found inside a
// `data` block's body become edges in a graph keyed by the enclosing block's
// `<type>.<name>`; refs found anywhere else are roots. The returned set is
// the BFS closure from those roots — so unreachable cycles between data
// sources are pruned, not kept alive by their own mutual references.
func (p *Pruner) reachableDataKeys() map[string]bool {
	edges := map[string][]string{}
	roots := map[string]bool{}
	for _, f := range p.files {
		for _, blk := range f.Body().Blocks() {
			refs := collectRefsInBody(blk.Body())
			labels := blk.Labels()
			if blk.Type() == "data" && len(labels) >= 2 {
				owner := labels[0] + "." + labels[1]
				edges[owner] = append(edges[owner], refs...)
				continue
			}
			for _, k := range refs {
				roots[k] = true
			}
		}
	}
	reached := map[string]bool{}
	queue := make([]string, 0, len(roots))
	for k := range roots {
		reached[k] = true
		queue = append(queue, k)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, next := range edges[n] {
			if reached[next] {
				continue
			}
			reached[next] = true
			queue = append(queue, next)
		}
	}
	return reached
}

func collectRefsInBody(body *hclwrite.Body) []string {
	var out []string
	for _, attr := range body.Attributes() {
		out = append(out, findDataRefs(attr.Expr().BuildTokens(nil))...)
	}
	for _, blk := range body.Blocks() {
		out = append(out, collectRefsInBody(blk.Body())...)
	}
	return out
}

// findDataRefs returns the `<type>.<name>` keys referenced via
// `data.<type>.<name>` in the given token sequence.
func findDataRefs(tokens hclwrite.Tokens) []string {
	var out []string
	seen := map[string]bool{}
	for i := 0; i < len(tokens); i++ {
		if !isDataRefStart(tokens, i) {
			continue
		}
		key := string(tokens[i+2].Bytes) + "." + string(tokens[i+4].Bytes)
		if !seen[key] {
			out = append(out, key)
			seen[key] = true
		}
		i += 4
	}
	return out
}

// isDataRefStart reports whether tokens[i:i+5] is `data.<ident>.<ident>` and
// is not preceded by `.` (which would make it part of a longer attribute
// access like `foo.data.x.y`).
func isDataRefStart(tokens hclwrite.Tokens, i int) bool {
	if i+4 >= len(tokens) {
		return false
	}
	t0, t1, t2, t3, t4 := tokens[i], tokens[i+1], tokens[i+2], tokens[i+3], tokens[i+4]
	if t0.Type != hclsyntax.TokenIdent || string(t0.Bytes) != "data" {
		return false
	}
	if t1.Type != hclsyntax.TokenDot {
		return false
	}
	if t2.Type != hclsyntax.TokenIdent {
		return false
	}
	if t3.Type != hclsyntax.TokenDot {
		return false
	}
	if t4.Type != hclsyntax.TokenIdent {
		return false
	}
	if i > 0 && tokens[i-1].Type == hclsyntax.TokenDot {
		return false
	}
	return true
}

// removeUnusedDataBlocks removes every `data "<type>" "<name>"` block whose
// `<type>.<name>` key is absent from `used`.
func (p *Pruner) removeUnusedDataBlocks(used, changedFiles map[string]bool) {
	for path, f := range p.files {
		body := f.Body()
		for _, blk := range body.Blocks() {
			if blk.Type() != "data" {
				continue
			}
			labels := blk.Labels()
			if len(labels) < 2 {
				continue
			}
			key := labels[0] + "." + labels[1]
			if used[key] {
				continue
			}
			body.RemoveBlock(blk)
			changedFiles[path] = true
		}
	}
}

func (p *Pruner) writeOut(inPlace bool, changedFiles map[string]bool) error {
	paths := make([]string, 0, len(p.files))
	for path := range p.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if !changedFiles[path] {
			continue
		}
		f := p.files[path]
		body := f.Bytes()
		if !inPlace {
			if _, err := fmt.Fprintf(p.Out, "### %s ###\n%s", path, body); err != nil {
				return err
			}
			continue
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
