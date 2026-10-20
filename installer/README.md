# Rey's Retreat Mod Downloader (CLI prototype)

A standalone Go CLI tool that downloads the mod JARs listed in a selected Rey's Retreat release into a folder. It does **not** install a Minecraft instance.

## Features

- Resolves `latest` to the newest stable GitHub release containing both Standard and NVIDIA editions.
- Supports Standard and NVIDIA edition manifests.
- Downloads only `.jar` files listed in the selected modpack manifest into the chosen folder.
- Does not extract configs, resource packs, shader packs, or `overrides/`.
- Verifies SHA-1/SHA-512 before accepting downloads and skips already-valid files.
- Optionally downloads Biomes O' Plenty and Tough As Nails from Modrinth, selecting a compatible Minecraft 1.21.1 + Fabric version and resolving required Modrinth dependencies.
- Refuses to use a non-empty destination folder.
- Does not install Java, Minecraft, Fabric Loader, or a launcher profile.

The selected release's `.mrpack` is downloaded to a temporary file solely to read its mod manifest; it is deleted afterward. The destination folder contains only mod JARs.

## Build on Windows

Install Go 1.24 or newer, then run in this directory:

```powershell
go test ./...
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-s -w" -o reys-retreat-mod-downloader.exe .
```

## Usage

Interactive:

```powershell
.\reys-retreat-mod-downloader.exe
```

Choose latest stable release explicitly:

```powershell
.\reys-retreat-mod-downloader.exe -version latest
```

Choose edition and destination:

```powershell
.\reys-retreat-mod-downloader.exe -edition standard -dir "D:\\Downloads\\Reys-Retreat-Mods"
```

Include optional mods without prompts:

```powershell
.\reys-retreat-mod-downloader.exe -edition standard -biomes-o-plenty -tough-as-nails
```

List stable releases that contain both editions:

```powershell
.\reys-retreat-mod-downloader.exe -list
```

## Prototype limitations

- Downloads the `.mrpack` manifest archive temporarily; only JARs are written into the destination folder.
- This does not launch Minecraft and does not install the mods into an existing profile.
- Downloads are not resumable. If a download fails, rerun the tool after removing the partial output folder.
- Before publishing, review each optional mod project's compatibility and distribution/API terms.
