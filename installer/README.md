# Rey's Retreat Installer (CLI prototype)

A standalone Go installer prototype for the Rey's Retreat modpack. It downloads official pack releases from GitHub and resolves optional mods through the Modrinth API.

## Features

- Resolves `latest` to the newest stable GitHub release that contains both Standard and NVIDIA editions.
- Supports Standard and NVIDIA editions.
- Downloads indexed files and extracts `overrides/` from the `.mrpack` archive.
- Verifies supported SHA-1/SHA-512 hashes before accepting downloads.
- Skips files that already match the expected hashes.
- Offers optional Biomes O' Plenty and Tough As Nails, selecting a compatible Fabric/Minecraft 1.21.1 version from Modrinth and installing required Modrinth dependencies.
- Refuses to install into a non-empty directory.
- Does not install Java, Minecraft Launcher, or Fabric Loader itself.

## Build on Windows

Install Go 1.24 or newer, then run in this directory:

```powershell
go test ./...
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-s -w" -o reys-retreat-installer.exe .
```

## Usage

Interactive (asks edition, destination and optional mods):

```powershell
.\reys-retreat-installer.exe
```

Use latest stable release explicitly:

```powershell
.\reys-retreat-installer.exe -version latest
```

Choose edition and destination without the edition prompt:

```powershell
.\reys-retreat-installer.exe -version latest -edition standard -dir "D:\\Games\\Reys-Retreat"
```

Install optional mods without prompts:

```powershell
.\reys-retreat-installer.exe -edition standard -biomes-o-plenty -tough-as-nails
```

List stable releases that contain both editions:

```powershell
.\reys-retreat-installer.exe -list
```

## Important prototype notes

- This creates pack files in a folder; it does not register a profile in a launcher or install the game runtime.
- The optional mods are resolved live from Modrinth. Always review their project pages, compatibility and licensing/permissions before publishing the installer as a public distribution method.
- The first version has no GUI, resumable downloads, signed manifest, or rollback transaction. A failed install can leave partial files in the chosen new directory; remove that directory and retry.
