# Rey's Retreat — tooling update

This is a **patch bundle**, not a replacement for the whole modpack repository. Copy the included files into the root of the existing project, reviewing/replacing files as described below.

## Intended layout

```text
reys-retreat-pack/
├── incoming/
│   └── processed/             # handled source JARs are archived here
├── mods/                      # shared mods and .pw.toml metadata
├── extra/
│   └── nvidia/
│       └── nvidium.pw.toml     # NVIDIA-only mod metadata; never kept in mods/
├── dist/                      # built .mrpack files
├── main.go
├── go.mod
├── build.bat
├── pack.toml
└── index.toml
```

Keep the existing `pack.toml`, `index.toml`, `config/`, `mods/` and `.github/` contents. Do not replace those with this patch. The archive's `main.go` and `build.bat` replace existing files; `main_test.go` is an optional test file. Keep your existing `go.mod` and `.gitignore`. The included GitHub Actions workflow is an optional replacement for `.github/workflows/release.yml`.

## 1. Install the files

1. Back up the project folder or commit your current work in Git.
2. Copy `main.go`, `go.mod`, and `build.bat` from this patch into the root of your existing project.
3. Keep the existing `extra/nvidia/nvidium.pw.toml` in place. If it currently has another name/path, adjust the build script to match it.
4. Optionally copy `main_test.go` into the root if you want the included unit tests.
5. If you use GitHub tag releases, replace `.github/workflows/release.yml` with the included workflow; it builds both editions in isolated workspaces.
6. Create `incoming/` if it does not exist.
7. Keep disabled shared mods as `mods/<name>.pw.toml.disabled`. The active metadata file must not also remain under its `.pw.toml` name.

## 2. Import downloaded mods

Put all downloaded `.jar` files directly in `incoming/`, including NVIDIA-only mods. By default, the importer automatically routes the Modrinth project slug `nvidium` to `extra/nvidia/`; use `-nvidia-slugs` to configure other NVIDIA-only projects.

Run from the project root:

```bat
go run . import
```

The importer computes each JAR's SHA-1 and asks Modrinth whether that exact file is known. For a match it writes a `.pw.toml` in `mods/` using the Modrinth project's download URL and SHA-512 checksum, then moves the source JAR to `incoming/processed/`.

If a JAR cannot be matched exactly on Modrinth, it stays in `incoming/` and is reported as `MANUAL`. Do not guess the download URL or checksum. Resolve the source manually or leave the file out of the pack until its source is known.

The importer routes the Modrinth project slug `nvidium` to `extra/nvidia/` automatically. Other NVIDIA-only projects can be added to `-nvidia-slugs`; do not leave an active NVIDIA-only `.pw.toml` in `mods/`. If a project is already listed in its target edition, the importer skips it rather than creating a duplicate active version.

Files are not deleted. Handled JARs are archived in `incoming/processed/`; if a metadata write or move fails, the source file is left available for recovery.

## 3. Disabled mods

For a mod to be excluded, rename the active metadata file, for example:

```text
mods/example-mod.pw.toml
```

to:

```text
mods/example-mod.pw.toml.disabled
```

Do not keep both files. The build script removes `*.pw.toml.disabled` from each temporary workspace before running `packwiz refresh`, so these files remain in your working project but are not included in either export. You do not need to run `packwiz refresh` in the source directory before building.

## 4. Build both editions

Run:

```bat
build.bat 0.3.1
```

The script copies the project into two isolated temporary workspaces, excluding `.git`, `dist/`, and `incoming/`. It runs `packwiz refresh` and exports Standard in one workspace. In the other workspace only, it copies `extra/nvidia/nvidium.pw.toml` to `mods/nvidium.pw.toml`, refreshes, and exports NVIDIA. The source project is not modified by either build.

Outputs:

```text
dist/reys-retreat-standard-0.3.1.mrpack
dist/reys-retreat-nvidia-0.3.1.mrpack
```

The build script requires `packwiz` to be installed and available in `PATH`. It intentionally keeps the temporary workspace if Packwiz fails, so the failure can be inspected.

## 5. Existing GitHub Actions workflow

The included `.github/workflows/release.yml` uses isolated temporary workspaces for tagged releases. Copy it over your current workflow if you want the same behavior in GitHub Actions.

## Safety checks before publishing

- Run `packwiz refresh` and `packwiz list` and inspect the active mods.
- Confirm Nvidium is absent from Standard and present in NVIDIA.
- Confirm the two `.disabled` mods are absent from both builds.
- Launch both exported `.mrpack` files in a clean Modrinth profile and test the game.
- Keep the `incoming/processed/` source JARs until the release has been tested.

### Automatic NVIDIA routing

By default, imports whose Modrinth project slug is `nvidium` are written directly to `extra/nvidia/`, not `mods/`. You can configure additional NVIDIA-only projects, for example:

```bat
go run . import -nvidia-slugs nvidium,another-nvidia-only-mod
```

Use the exact Modrinth project slug, comma-separated. This routing only applies to the listed projects; all other imported mods go to `mods/`.
