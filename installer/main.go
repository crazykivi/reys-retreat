package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPI = "https://api.github.com/repos/crazykivi/reys-retreat/releases"
	modrinthAPI = "https://api.modrinth.com/v2"
	gameVersion = "1.21.1"
	userAgent = "ReysRetreatInstaller/0.1.0 (https://github.com/crazykivi/reys-retreat)"
)

type release struct {
	TagName string `json:"tag_name"`
	Name string `json:"name"`
	Draft bool `json:"draft"`
	Prerelease bool `json:"prerelease"`
	HTMLURL string `json:"html_url"`
	Assets []releaseAsset `json:"assets"`
}
type releaseAsset struct {
	Name string `json:"name"`
	Size int64 `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
type mrpackIndex struct {
	Name string `json:"name"`
	VersionID string `json:"versionId"`
	Dependencies map[string]string `json:"dependencies"`
	Files []mrpackFile `json:"files"`
}
type mrpackFile struct {
	Path string `json:"path"`
	Hashes map[string]string `json:"hashes"`
	Downloads []string `json:"downloads"`
	Env map[string]string `json:"env"`
}
type modVersion struct {
	ID string `json:"id"`
	ProjectID string `json:"project_id"`
	Name string `json:"name"`
	VersionNumber string `json:"version_number"`
	GameVersions []string `json:"game_versions"`
	Loaders []string `json:"loaders"`
	VersionType string `json:"version_type"`
	Files []modFile `json:"files"`
	Dependencies []modDependency `json:"dependencies"`
}
type modFile struct {
	Hashes map[string]string `json:"hashes"`
	URL string `json:"url"`
	Filename string `json:"filename"`
	Primary bool `json:"primary"`
	Size int64 `json:"size"`
}
type modDependency struct {
	VersionID string `json:"version_id"`
	ProjectID string `json:"project_id"`
	FileName string `json:"file_name"`
	DependencyType string `json:"dependency_type"`
}

var httpClient = &http.Client{Timeout: 90 * time.Second}
var stdinReader = bufio.NewReader(os.Stdin)

func main() {
	versionFlag := flag.String("version", "latest", "release tag (default: latest)")
	editionFlag := flag.String("edition", "", "standard or nvidia")
	dirFlag := flag.String("dir", "", "installation directory")
	bopFlag := flag.Bool("biomes-o-plenty", false, "also install Biomes O' Plenty")
	tanFlag := flag.Bool("tough-as-nails", false, "also install Tough As Nails")
	listFlag := flag.Bool("list", false, "list available stable releases and exit")
	flag.Parse()

	fmt.Println("Rey's Retreat Installer")
	fmt.Println("-----------------------")
	if runtime.GOOS != "windows" {
		fmt.Println("Note: this is a cross-platform Go CLI; the first target is Windows.")
	}

	releases, err := getReleases()
	if err != nil {
		fatalf("Cannot read GitHub releases: %v", err)
	}
	if *listFlag {
		for _, r := range releases {
			fmt.Printf("%-12s %s\n", r.TagName, r.Name)
		}
		return
	}
	if len(releases) == 0 {
		fatalf("No stable GitHub releases with Standard/NVIDIA assets were found.")
	}

	version := *versionFlag
	if version == "" || strings.EqualFold(version, "latest") {
		version = releases[0].TagName
	}
	var selected *release
	for i := range releases {
		if releases[i].TagName == version {
			selected = &releases[i]
			break
		}
	}
	if selected == nil {
		fmt.Println("Available stable releases:")
		for _, r := range releases { fmt.Printf("  %s - %s\n", r.TagName, r.Name) }
		fatalf("Release %q was not found. Use -version latest or an exact tag.", version)
	}

	edition := strings.ToLower(strings.TrimSpace(*editionFlag))
	if edition == "" {
		edition = askChoice("Choose edition: 1) Standard (recommended)  2) NVIDIA (Nvidium)", map[string]string{"1":"standard","2":"nvidia","standard":"standard","nvidia":"nvidia"})
	}
	if edition != "standard" && edition != "nvidia" {
		fatalf("Edition must be standard or nvidia.")
	}
	assetName := fmt.Sprintf("reys-retreat-%s-%s.mrpack", edition, strings.TrimPrefix(selected.TagName, "v"))
	assetURL := ""
	for _, a := range selected.Assets {
		if a.Name == assetName { assetURL = a.BrowserDownloadURL; break }
	}
	if assetURL == "" {
		fatalf("Release %s does not contain asset %s.", selected.TagName, assetName)
	}

	dest := strings.TrimSpace(*dirFlag)
	if dest == "" {
		defaultDir := filepath.Join(".", "Reys-Retreat-"+strings.TrimPrefix(selected.TagName, "v")+"-"+strings.ToUpper(edition))
		fmt.Printf("Installation directory [%s]: ", defaultDir)
		if line, _ := bufio.NewReader(os.Stdin).ReadString('\n'); strings.TrimSpace(line) != "" { dest = strings.TrimSpace(line) } else { dest = defaultDir }
	}
	dest, err = filepath.Abs(dest)
	if err != nil { fatalf("Invalid installation path: %v", err) }
	if _, err := os.Stat(dest); err == nil {
		if !isDirectoryEmpty(dest) {
			fmt.Printf("Directory %s already exists and is not empty.\n", dest)
			choice := askChoice("Continue and merge files? 1) No (recommended)  2) Yes", map[string]string{"1":"no","2":"yes","no":"no","yes":"yes"})
			if choice != "yes" { fmt.Println("Cancelled; existing files were not changed."); return }
		}
	} else if !errors.Is(err, os.ErrNotExist) { fatalf("Cannot access destination: %v", err) }

	fmt.Printf("\nRelease: %s\nEdition: %s\nTarget: %s\n", selected.TagName, edition, dest)
	fmt.Println("Downloading modpack manifest...")
	packPath := filepath.Join(os.TempDir(), "reys-retreat-"+strings.TrimPrefix(selected.TagName, "v")+"-"+edition+".mrpack")
	if err := downloadToFile(assetURL, packPath, nil, 0); err != nil { fatalf("Modpack download failed: %v", err) }
	defer os.Remove(packPath)

	if err := installMrpack(packPath, dest); err != nil { fatalf("Installation failed: %v", err) }
	modsDir := filepath.Join(dest, "mods")
	bop, tan := *bopFlag, *tanFlag
	if !flagWasPassed("biomes-o-plenty") { bop = askYesNo("Install optional Biomes O' Plenty? [y/N]") }
	if !flagWasPassed("tough-as-nails") { tan = askYesNo("Install optional Tough As Nails? [y/N]") }
	installed := map[string]bool{}
	if bop { if err := installModBySlug("biomes-o-plenty", modsDir, installed); err != nil { fatalf("Biomes O' Plenty: %v", err) } }
	if tan { if err := installModBySlug("tough-as-nails", modsDir, installed); err != nil { fatalf("Tough As Nails: %v", err) } }

	fmt.Println("\nInstallation files are ready.")
	fmt.Println("This tool installs the pack files; it does not install Java or a Minecraft launcher.")
	fmt.Println("Use a launcher configured for Minecraft 1.21.1 + Fabric to launch this instance.")
}

func getReleases() ([]release, error) {
	var all []release
	if err := getJSON(githubAPI+"?per_page=100", &all); err != nil { return nil, err }
	out := make([]release, 0, len(all))
	for _, r := range all {
		if r.Draft || r.Prerelease { continue }
		hasStandard, hasNvidia := false, false
		for _, a := range r.Assets {
			if strings.HasPrefix(a.Name, "reys-retreat-standard-") && strings.HasSuffix(a.Name, ".mrpack") { hasStandard = true }
			if strings.HasPrefix(a.Name, "reys-retreat-nvidia-") && strings.HasSuffix(a.Name, ".mrpack") { hasNvidia = true }
		}
		if hasStandard && hasNvidia { out = append(out, r) }
	}
	return out, nil
}

func installMrpack(packPath, dest string) error {
	zr, err := zip.OpenReader(packPath)
	if err != nil { return fmt.Errorf("invalid .mrpack archive: %w", err) }
	defer zr.Close()
	var index mrpackIndex
	var foundIndex bool
	for _, f := range zr.File {
		if f.Name == "modrinth.index.json" {
			rc, err := f.Open(); if err != nil { return err }
			err = json.NewDecoder(io.LimitReader(rc, 16<<20)).Decode(&index); _ = rc.Close()
			if err != nil { return fmt.Errorf("read modrinth.index.json: %w", err) }
			foundIndex = true; break
		}
	}
	if !foundIndex { return errors.New("archive does not contain modrinth.index.json") }
	if index.Dependencies["minecraft"] != gameVersion { return fmt.Errorf("pack targets Minecraft %q; this installer only supports %s", index.Dependencies["minecraft"], gameVersion) }
	if index.Dependencies["fabric-loader"] == "" { return errors.New("pack does not declare a Fabric loader") }

	if err := os.MkdirAll(dest, 0755); err != nil { return err }
	fmt.Printf("Installing %d indexed files...\n", len(index.Files))
	for i, item := range index.Files {
		if item.Env["client"] == "unsupported" { continue }
		rel, err := safeRelative(item.Path); if err != nil { return fmt.Errorf("unsafe manifest path %q: %w", item.Path, err) }
		target := filepath.Join(dest, rel)
		if fileMatchesHashes(target, item.Hashes) { fmt.Printf("[%d/%d] already valid: %s\n", i+1, len(index.Files), item.Path); continue }
		if len(item.Downloads) == 0 { return fmt.Errorf("%s has no download URLs", item.Path) }
		var lastErr error
		for _, u := range item.Downloads {
			if err := downloadToFile(u, target, item.Hashes, 0); err == nil { lastErr = nil; break } else { lastErr = err }
		}
		if lastErr != nil { return fmt.Errorf("download %s: %w", item.Path, lastErr) }
		fmt.Printf("[%d/%d] downloaded: %s\n", i+1, len(index.Files), item.Path)
	}
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "overrides/") || f.FileInfo().IsDir() { continue }
		rel, err := safeRelative(strings.TrimPrefix(f.Name, "overrides/")); if err != nil { return fmt.Errorf("unsafe override path %q: %w", f.Name, err) }
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { return err }
		if _, err := os.Stat(target); err == nil { continue } else if !errors.Is(err, os.ErrNotExist) { return err }
		rc, err := f.Open(); if err != nil { return err }
		if err := writeLimited(target, rc, 256<<20); err != nil { _ = rc.Close(); return fmt.Errorf("extract override %s: %w", rel, err) }
		_ = rc.Close()
	}
	fmt.Printf("Minecraft %s / Fabric %s\n", index.Dependencies["minecraft"], index.Dependencies["fabric-loader"])
	return nil
}

func installModBySlug(slug, modsDir string, installed map[string]bool) error {
	if installed[slug] { return nil }
	installed[slug] = true
	escaped := url.PathEscape(slug)
	var versions []modVersion
	u := modrinthAPI + "/project/" + escaped + "/version?game_versions=%5B%22" + gameVersion + "%22%5D&loaders=%5B%22fabric%22%5D"
	if err := getJSON(u, &versions); err != nil { return err }
	var chosen *modVersion
	for i := range versions {
		v := &versions[i]
		if !contains(v.GameVersions, gameVersion) || !contains(v.Loaders, "fabric") { continue }
		if v.VersionType == "release" { chosen = v; break }
	}
	if chosen == nil {
		for i := range versions { if contains(versions[i].GameVersions, gameVersion) && contains(versions[i].Loaders, "fabric") { chosen = &versions[i]; break } }
	}
	if chosen == nil { return fmt.Errorf("Modrinth has no compatible Fabric version for Minecraft %s", gameVersion) }
	if len(chosen.Files) == 0 { return fmt.Errorf("Modrinth version %s has no files", chosen.ID) }
	file := chosen.Files[0]
	for _, f := range chosen.Files { if f.Primary { file = f; break } }
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".jar") { return fmt.Errorf("unexpected file type %q", file.Filename) }
	if err := os.MkdirAll(modsDir, 0755); err != nil { return err }
	target := filepath.Join(modsDir, filepath.Base(file.Filename))
	if fileMatchesHashes(target, file.Hashes) {
		fmt.Printf("Optional mod already valid: %s %s\n", chosen.Name, chosen.VersionNumber)
	} else {
		if err := downloadToFile(file.URL, target, file.Hashes, file.Size); err != nil { return fmt.Errorf("download %s: %w", chosen.Name, err) }
		fmt.Printf("Added optional mod: %s %s\n", chosen.Name, chosen.VersionNumber)
	}
	for _, dep := range chosen.Dependencies {
		if dep.DependencyType != "required" { continue }
		if dep.VersionID != "" { if err := installModVersionByID(dep.VersionID, modsDir, installed); err != nil { return fmt.Errorf("required dependency for %s: %w", chosen.Name, err) } } else if dep.ProjectID != "" {
			if err := installModByProjectID(dep.ProjectID, modsDir, installed); err != nil { return fmt.Errorf("required dependency for %s: %w", chosen.Name, err) }
		}
	}
	return nil
}

func installModByProjectID(projectID, modsDir string, installed map[string]bool) error {
	if installed[projectID] { return nil }; installed[projectID] = true
	var versions []modVersion
	u := modrinthAPI + "/project/" + url.PathEscape(projectID) + "/version?game_versions=%5B%22" + gameVersion + "%22%5D&loaders=%5B%22fabric%22%5D"
	if err := getJSON(u, &versions); err != nil { return err }
	for i := range versions { if contains(versions[i].GameVersions, gameVersion) && contains(versions[i].Loaders, "fabric") && versions[i].VersionType == "release" { return saveModVersion(&versions[i], modsDir, installed) } }
	for i := range versions { if contains(versions[i].GameVersions, gameVersion) && contains(versions[i].Loaders, "fabric") { return saveModVersion(&versions[i], modsDir, installed) } }
	return fmt.Errorf("no compatible version found for dependency %s", projectID)
}
func installModVersionByID(id, modsDir string, installed map[string]bool) error {
	if installed[id] { return nil }; installed[id] = true
	var v modVersion
	if err := getJSON(modrinthAPI+"/version/"+url.PathEscape(id), &v); err != nil { return err }
	if !contains(v.GameVersions, gameVersion) || !contains(v.Loaders, "fabric") { return fmt.Errorf("dependency version %s is incompatible", id) }
	return saveModVersion(&v, modsDir, installed)
}
func saveModVersion(v *modVersion, modsDir string, installed map[string]bool) error {
	if len(v.Files) == 0 { return fmt.Errorf("version %s has no files", v.ID) }
	file := v.Files[0]; for _, f := range v.Files { if f.Primary { file = f; break } }
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".jar") { return fmt.Errorf("unexpected dependency file %q", file.Filename) }
	if err := os.MkdirAll(modsDir, 0755); err != nil { return err }
	target := filepath.Join(modsDir, filepath.Base(file.Filename))
	if !fileMatchesHashes(target, file.Hashes) {
		if err := downloadToFile(file.URL, target, file.Hashes, file.Size); err != nil { return err }
		fmt.Printf("Added required dependency: %s %s\n", v.Name, v.VersionNumber)
	}
	for _, dep := range v.Dependencies { if dep.DependencyType == "required" { if dep.VersionID != "" { if err := installModVersionByID(dep.VersionID, modsDir, installed); err != nil { return err } } else if dep.ProjectID != "" { if err := installModByProjectID(dep.ProjectID, modsDir, installed); err != nil { return err } } } }
	return nil
}

func downloadToFile(rawURL, target string, hashes map[string]string, expectedSize int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil { return err }
	tmp := target + ".part"
	_ = os.Remove(tmp)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil); if err != nil { return err }
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req); if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("HTTP %s from %s", resp.Status, rawURL) }
	if expectedSize > 0 && resp.ContentLength > 0 && resp.ContentLength != expectedSize { return fmt.Errorf("size mismatch: expected %d, server reports %d", expectedSize, resp.ContentLength) }
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644); if err != nil { return err }
	var writers []io.Writer
	sha1h, sha512h := sha1.New(), sha512.New()
	writers = append(writers, out, sha1h, sha512h)
	n, copyErr := io.Copy(io.MultiWriter(writers...), io.LimitReader(resp.Body, 2<<30))
	closeErr := out.Close()
	if copyErr != nil { _ = os.Remove(tmp); return copyErr }
	if closeErr != nil { _ = os.Remove(tmp); return closeErr }
	if expectedSize > 0 && n != expectedSize { _ = os.Remove(tmp); return fmt.Errorf("size mismatch: expected %d bytes, got %d", expectedSize, n) }
	if err := verifyHashValues(hashes, sha1h, sha512h); err != nil { _ = os.Remove(tmp); return err }
	if err := os.Rename(tmp, target); err != nil { _ = os.Remove(tmp); return err }
	return nil
}
func verifyHashValues(expected map[string]string, h1, h512 hash.Hash) error {
	if len(expected) == 0 { return errors.New("no expected hash provided; refusing unverified download") }
	for algorithm, want := range expected {
		var got string
		switch strings.ToLower(algorithm) {
		case "sha1": got = hex.EncodeToString(h1.Sum(nil))
		case "sha512": got = hex.EncodeToString(h512.Sum(nil))
		default: continue
		}
		if got != "" && !strings.EqualFold(got, want) { return fmt.Errorf("%s mismatch: expected %s, got %s", algorithm, want, got) }
	}
	if expected["sha1"] == "" && expected["sha512"] == "" { return errors.New("manifest contains no supported SHA-1/SHA-512 hash") }
	return nil
}
func fileMatchesHashes(filename string, hashes map[string]string) bool {
	if len(hashes) == 0 { return false }
	f, err := os.Open(filename); if err != nil { return false }; defer f.Close()
	h1, h512 := sha1.New(), sha512.New()
	if _, err := io.Copy(io.MultiWriter(h1, h512), f); err != nil { return false }
	return verifyHashValues(hashes, h1, h512) == nil
}
func safeRelative(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') { return "", errors.New("absolute or empty path") }
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") { return "", errors.New("path traversal") }
	return filepath.FromSlash(clean), nil
}
func writeLimited(target string, src io.Reader, limit int64) error {
	tmp := target + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); if err != nil { return err }
	n, copyErr := io.Copy(out, io.LimitReader(src, limit+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > limit { _ = os.Remove(tmp); if copyErr != nil { return copyErr }; if closeErr != nil { return closeErr }; return errors.New("override file exceeds extraction limit") }
	if err := os.Rename(tmp, target); err != nil { _ = os.Remove(tmp); return err }
	return nil
}
func getJSON(rawURL string, target any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil); if err != nil { return err }
	req.Header.Set("User-Agent", userAgent); req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req); if err != nil { return err }; defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048)); return fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(body))) }
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(target)
}
func contains(values []string, wanted string) bool { for _, v := range values { if v == wanted { return true } }; return false }
func isDirectoryEmpty(dir string) bool {
	entries, err := os.ReadDir(dir); if errors.Is(err, os.ErrNotExist) { return true }; return err == nil && len(entries) == 0
}
func flagWasPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) { if f.Name == name { found = true } })
	return found
}
func askYesNo(prompt string) bool {
	for {
		fmt.Print(prompt + " ")
		line, _ := stdinReader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes": return true
		case "", "n", "no": return false
		default: fmt.Println("Please answer y or n.")
		}
	}
}
func askChoice(prompt string, options map[string]string) string {
	for {
		fmt.Print(prompt + ": ")
		line, _ := stdinReader.ReadString('\n')
		if value, ok := options[strings.ToLower(strings.TrimSpace(line))]; ok { return value }
		fmt.Println("Invalid choice; try again.")
	}
}
func fatalf(format string, args ...any) { fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...); os.Exit(1) }
