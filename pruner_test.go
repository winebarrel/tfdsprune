package tfdsprune_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winebarrel/tfdsprune"
)

func TestPrune_Golden(t *testing.T) {
	cases := []string{
		"basic",
		"transitive",
		"across-files",
		"all-used",
		"nested-ref",
		"cycle",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := copyInputToTemp(t, filepath.Join("testdata", name, "input"))
			p := tfdsprune.NewPruner(tmp)
			require.NoError(t, p.Prune(true))
			compareDir(t, tmp, filepath.Join("testdata", name, "expected"))
		})
	}
}

func TestPrune_ParseError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/parse-error/input")
	p := tfdsprune.NewPruner(tmp)
	err := p.Prune(true)
	require.Error(t, err)
}

func TestPrune_StdoutMode(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic/input")
	var buf bytes.Buffer
	p := tfdsprune.NewPruner(tmp)
	p.Out = &buf
	require.NoError(t, p.Prune(false))
	assert.Contains(t, buf.String(), `data "aws_ami" "used"`)
	assert.NotContains(t, buf.String(), `data "aws_vpc" "unused"`)
	got, err := os.ReadFile(filepath.Join(tmp, "main.tf"))
	require.NoError(t, err)
	want, err := os.ReadFile("testdata/basic/input/main.tf")
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), "file must not be modified in stdout mode")
}

func TestPrune_StdoutNoChange(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/all-used/input")
	var buf bytes.Buffer
	p := tfdsprune.NewPruner(tmp)
	p.Out = &buf
	require.NoError(t, p.Prune(false))
	assert.Empty(t, buf.String())
}

func TestPrune_StdoutWriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic/input")
	p := tfdsprune.NewPruner(tmp)
	p.Out = failingWriter{}
	err := p.Prune(false)
	require.Error(t, err)
}

func TestPrune_GlobError(t *testing.T) {
	p := tfdsprune.NewPruner("[invalid")
	err := p.Prune(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "glob")
}

func TestPrune_LoadReadError(t *testing.T) {
	tmp := t.TempDir()
	// A directory named "trap.tf" makes os.ReadFile fail when load() iterates it.
	require.NoError(t, os.Mkdir(filepath.Join(tmp, "trap.tf"), 0o755))
	p := tfdsprune.NewPruner(tmp)
	err := p.Prune(true)
	require.Error(t, err)
}

func TestPrune_WriteError(t *testing.T) {
	tmp := copyInputToTemp(t, "testdata/basic/input")
	// Make the file read-only so os.WriteFile fails when the in-place rewrite
	// reopens it with O_WRONLY|O_TRUNC.
	require.NoError(t, os.Chmod(filepath.Join(tmp, "main.tf"), 0o444))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(tmp, "main.tf"), 0o644) })
	p := tfdsprune.NewPruner(tmp)
	err := p.Prune(true)
	require.Error(t, err)
}

func TestPrune_DataBlockMissingLabels(t *testing.T) {
	tmp := t.TempDir()
	// A `data` block with only one label is malformed but the HCL writer parses
	// it; the pruner should leave it alone rather than panic on labels[1].
	src := []byte(`data "only_one" {
  x = 1
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), src, 0o644))
	p := tfdsprune.NewPruner(tmp)
	require.NoError(t, p.Prune(true))
	got, err := os.ReadFile(filepath.Join(tmp, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, string(src), string(got))
}

// ----------------- unit tests for internal helpers -----------------

func TestIsDataRefStart_PrecededByDot(t *testing.T) {
	toks := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("foo")},
		{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("data")},
		{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("t")},
		{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("n")},
	}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 2))
}

func TestIsDataRefStart_TooShort(t *testing.T) {
	toks := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("data")},
		{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("t")},
	}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))
}

func TestIsDataRefStart_WrongTokenTypes(t *testing.T) {
	base := func() hclwrite.Tokens {
		return hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte("data")},
			{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
			{Type: hclsyntax.TokenIdent, Bytes: []byte("t")},
			{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
			{Type: hclsyntax.TokenIdent, Bytes: []byte("n")},
		}
	}

	// Non-"data" ident at position 0.
	toks := base()
	toks[0] = &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("not_data")}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))

	// Non-dot at position 1.
	toks = base()
	toks[1] = &hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))

	// Non-ident at position 2.
	toks = base()
	toks[2] = &hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))

	// Non-dot at position 3.
	toks = base()
	toks[3] = &hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))

	// Non-ident at position 4.
	toks = base()
	toks[4] = &hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")}
	assert.False(t, tfdsprune.IsDataRefStart(toks, 0))
}

func TestFindDataRefs_Deduplicates(t *testing.T) {
	src := []byte(`x = data.t.n.a + data.t.n.b + data.t.m.c`)
	f, diags := hclwrite.ParseConfig(src, "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	tokens := f.Body().GetAttribute("x").Expr().BuildTokens(nil)
	keys := tfdsprune.FindDataRefs(tokens)
	assert.ElementsMatch(t, []string{"t.n", "t.m"}, keys)
}

// ----------------- test helpers -----------------

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) { return 0, errors.New("boom") }

func copyInputToTemp(t *testing.T, srcDir string) string {
	t.Helper()
	tmp := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, ent.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(tmp, ent.Name()), data, 0o644))
	}
	return tmp
}

func compareDir(t *testing.T, gotDir, wantDir string) {
	t.Helper()
	entries, err := os.ReadDir(wantDir)
	require.NoError(t, err)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		got, err := os.ReadFile(filepath.Join(gotDir, ent.Name()))
		require.NoError(t, err)
		want, err := os.ReadFile(filepath.Join(wantDir, ent.Name()))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), "file %s", ent.Name())
	}
}
