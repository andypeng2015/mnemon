# Mnemon Harness Usage

These commands assume you built:

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

## 1. Install Agent Integration

Install memory and skill integration into the current project:

```sh
./mnemon-harness setup --host codex --memory --skills --project-root .
```

Use `--dry-run` to preview file changes:

```sh
./mnemon-harness setup --host codex --memory --skills --project-root . --dry-run
```

## 2. Run Local Mnemon

Start the local service used by the projected host skills:

```sh
./mnemon-harness local run
```

Inspect local state:

```sh
./mnemon-harness local status
./mnemon-harness status
```

## 3. Remote Workspace Sync

Connect a Remote Workspace:

```sh
./mnemon-harness sync connect my-workspace
```

Run one push or pull:

```sh
./mnemon-harness sync push --once
./mnemon-harness sync pull --once
```

Run background sync:

```sh
./mnemon-harness sync run --background
```

## 4. Validate Declarations

Repository maintainers can validate harness loop, host, and binding manifests:

```sh
make harness-validate
```

This is a development check, not part of the normal user workflow.
