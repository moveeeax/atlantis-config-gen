# atlantis-config-gen

> Stop hand-writing `atlantis.yaml` — derive it from your repo structure.

**Status:** 🚧 In development

## Overview

Generate an atlantis.yaml from a repository's Terraform directory layout.

## Features

- Discovers root modules (dirs with backend/providers)
- Detects workspaces and tfvars per environment
- Emits projects with autoplan `when_modified` including shared modules
- Dry-run diff against existing atlantis.yaml
- Configurable naming and workflow templates

## Stack

Go + HCL discovery.

## Usage

```bash
atlantis-config-gen --root . --out atlantis.yaml
```

## License

MIT
