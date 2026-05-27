# tfdsprune

[![CI](https://github.com/winebarrel/tfdsprune/actions/workflows/ci.yml/badge.svg)](https://github.com/winebarrel/tfdsprune/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/winebarrel/tfdsprune/branch/main/graph/badge.svg)](https://codecov.io/gh/winebarrel/tfdsprune)
[![AI Generated](https://img.shields.io/badge/AI%20Generated-Claude-orange?logo=anthropic)](https://claude.ai/claude-code)

`tfdsprune` removes unused `data "<type>" "<name>"` blocks from Terraform `.tf` files. A data source is considered "used" when its `data.<type>.<name>` is reachable from non-`data` configuration (resources, outputs, modules, …) — either directly or transitively through another reachable data source. Data sources only referenced by other unreachable data sources, including cycles, are pruned.

## Installation

```
brew install winebarrel/tfdsprune/tfdsprune
```

## Usage

```
Usage: tfdsprune [<dir>] [flags]

Remove unused data sources from Terraform .tf files.

Arguments:
  [<dir>]    Directory containing *.tf files (default: ".").

Flags:
  -h, --help        Show help.
  -i, --in-place    Write changes back to files instead of stdout.
      --version
```

By default the result is printed to stdout. Pass `-i` / `--in-place` to rewrite the files on disk.

## Example

```hcl
# main.tf
data "aws_ami" "used" {
  most_recent = true
  owners      = ["amazon"]
}
data "aws_vpc" "unused" {
  default = true
}
data "aws_subnet" "also_unused" {
  vpc_id = "vpc-123"
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
}
```

```sh
tfdsprune -i .
```

```hcl
# main.tf (rewritten)
data "aws_ami" "used" {
  most_recent = true
  owners      = ["amazon"]
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
}
```

## Reachability

Usage is computed as a graph reachability problem:

- **Roots**: every `data.<type>.<name>` reference found outside any `data` block.
- **Edges**: a `data` block whose body references another `data.<type>.<name>` contributes an edge from itself to the referenced data source.
- A data source is **kept** iff it is reachable from at least one root.

This means a chain of data sources (`data.A` → `data.B` → `data.C`) is preserved when any link is referenced from outside, but is fully pruned when nothing outside references it. Cycles (`data.A` ↔ `data.B`) with no external reference are also pruned — they don't keep themselves alive.

## Limitations

- Only files matching `*.tf` directly in the target directory are scanned. Subdirectories are not recursed.
- Files with HCL parse errors stop the run with a diagnostic; nothing is rewritten.
- Only top-level `data` blocks are pruned. Malformed `data` blocks (fewer than 2 labels) are left untouched.
