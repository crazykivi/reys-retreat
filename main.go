package main

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const userAgent = "reys-retreat-pack-tools/1.0 (Modrinth modpack maintenance)"

type modrinthFile struct {
	Filename string            `json:"filename"`
	URL      string            `json:"url"`
	Primary  bool              `json:"primary"`
	Hashes   map[string]string `json:"hashes"`
}

type modrinthVersion struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	Name       string         `json:"name"`
	VersionNum string         `json:"version_number"`
	Files      []modrinthFile `json:"files"`
}

type modrinthProject struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	ClientSide string `json:"client_side"`
	ServerSide string `json:"server_side"`
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "import":
			if err := runImport(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ERROR:", err)
				os.Exit(1)
			}
			return
		case "mrpack":
			if err := runMrpack(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "ERROR:", err)
				os.Exit(1)
			}
			return
		case "help", "-h", "--help":
			usage()
			return
		}
	}

	// Backwards compatibility with the original syntax:
	// go run . -out mods reys-retreat-0.3.0.mrpack
	if err := runMrpack(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`Rey's Retreat pack tools

Commands:
  go run . import                         Import .jar files from incoming/
  go run . import -in incoming -out mods  Set input and output folders
  go run . mrpack pack.mrpack             Convert a Modrinth pack to Packwiz metadata
  go run . -out mods pack.mrpack          Legacy form of the mrpack command

Import behavior:
  - Matches each JAR by SHA-1 against Modrinth's version-file API.
  - Creates a .pw.toml only when a matching Modrinth file is found.
  - Moves successfully imported JARs to incoming/processed/.
  - Leaves unknown or already-listed files in incoming/ for manual review; nothing is deleted.`)
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	inDir := fs.String("in", "incoming", "folder containing downloaded JAR files")
	outDir := fs.String("out", "mods", "folder where .pw.toml files are written")
	processedDir := fs.String("processed", "", "folder for handled JARs (default: <in>/processed)")
	nvidiaOut := fs.String("nvidia-out", "extra/nvidia", "folder for NVIDIA-only metadata")
	nvidiaSlugs := fs.String("nvidia-slugs", "nvidium", "comma-separated Modrinth slugs routed to NVIDIA-only folder")
	offline := fs.Bool("offline", false, "do not call Modrinth; report all JARs as unresolved")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("import does not accept positional arguments; use -in and -out")
	}
	if *processedDir == "" {
		*processedDir = filepath.Join(*inDir, "processed")
	}
	entries, err := os.ReadDir(*inDir)
	if err != nil {
		return fmt.Errorf("read incoming folder %q: %w", *inDir, err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	var imported, already, unresolved, failed int
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jar") {
			continue
		}
		path := filepath.Join(*inDir, entry.Name())
		fmt.Printf("\n[%s]\n", entry.Name())
		if *offline {
			fmt.Println("MANUAL: offline mode; no Modrinth lookup performed")
			unresolved++
			continue
		}
		hash, err := sha1File(path)
		if err != nil {
			fmt.Println("ERROR:", err)
			failed++
			continue
		}
		version, err := lookupVersionByHash(client, hash)
		if err != nil {
			if errors.Is(err, errNotFound) {
				fmt.Println("MANUAL: no exact SHA-1 match on Modrinth; file left in incoming/")
				unresolved++
			} else {
				fmt.Println("ERROR:", err, "— file left in incoming/")
				failed++
			}
			continue
		}
		project, err := fetchProject(client, version.ProjectID)
		if err != nil {
			fmt.Println("ERROR: matched version but could not read project metadata:", err, "— file left in incoming/")
			failed++
			continue
		}
		file, err := chooseFile(version.Files, hash)
		if err != nil {
			fmt.Println("ERROR:", err, "— file left in incoming/")
			failed++
			continue
		}
		targetDir := *outDir
		if isNvidiaOnly(project.Slug, *nvidiaSlugs) {
			targetDir = *nvidiaOut
		}
		if hasFilename(targetDir, entry.Name()) || hasProjectID(targetDir, project.ID) {
			fmt.Println("ALREADY: this filename or Modrinth project already exists in the target edition; no duplicate metadata was created. File left in incoming/ for review.")
			already++
			continue
		}
		name := project.Slug
		if name == "" {
			name = project.ID
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			fmt.Println("ERROR: create metadata folder:", err, "— file left in incoming/")
			failed++
			continue
		}
		metaName := safeSlug(name)
		if metaName == "" {
			metaName = project.ID
		}
		metaPath := filepath.Join(targetDir, metaName+".pw.toml")
		if _, statErr := os.Stat(metaPath); statErr == nil {
			metaPath = filepath.Join(targetDir, metaName+"-"+project.ID+".pw.toml")
		}
		if _, statErr := os.Stat(metaPath); statErr == nil {
			fmt.Println("ALREADY: metadata file exists:", filepath.Base(metaPath), "— file left in incoming/")
			already++
			continue
		}
		meta, err := makePackwizMeta(project, version, file, entry.Name())
		if err != nil {
			fmt.Println("ERROR:", err, "— file left in incoming/")
			failed++
			continue
		}
		if err := writeExclusive(metaPath, []byte(meta)); err != nil {
			fmt.Println("ERROR: write metadata:", err, "— file left in incoming/")
			failed++
			continue
		}
		if err := moveToProcessed(path, *processedDir); err != nil {
			_ = os.Remove(metaPath)
			fmt.Println("ERROR: metadata was rolled back because JAR could not be moved:", err)
			failed++
			continue
		}
		fmt.Printf("IMPORTED: %s -> %s (Modrinth project: %s; version: %s)\n", entry.Name(), filepath.Base(metaPath), project.Title, version.VersionNum)
		imported++
	}

	fmt.Println("\nImport summary")
	fmt.Printf("  Imported:   %d\n  Already:    %d\n  Manual:     %d\n  Failed:     %d\n", imported, already, unresolved, failed)
	if unresolved > 0 || failed > 0 {
		return fmt.Errorf("import finished with %d unresolved and %d failed file(s); inspect the report above", unresolved, failed)
	}
	return nil
}

var errNotFound = errors.New("not found on Modrinth")

func lookupVersionByHash(c *http.Client, sha1Hash string) (modrinthVersion, error) {
	var v modrinthVersion
	status, body, err := getAPI(c, "https://api.modrinth.com/v2/version_file/"+sha1Hash+"?algorithm=sha1")
	if err != nil {
		return v, err
	}
	if status == http.StatusNotFound {
		return v, errNotFound
	}
	if status != http.StatusOK {
		return v, fmt.Errorf("Modrinth hash lookup returned HTTP %d: %s", status, apiMessage(body))
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return v, fmt.Errorf("decode Modrinth version response: %w", err)
	}
	if v.ID == "" || v.ProjectID == "" || len(v.Files) == 0 {
		return v, errors.New("Modrinth response is missing version, project, or file information")
	}
	return v, nil
}

func fetchProject(c *http.Client, id string) (modrinthProject, error) {
	var p modrinthProject
	status, body, err := getAPI(c, "https://api.modrinth.com/v2/project/"+id)
	if err != nil {
		return p, err
	}
	if status != http.StatusOK {
		return p, fmt.Errorf("Modrinth project lookup returned HTTP %d: %s", status, apiMessage(body))
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return p, fmt.Errorf("decode Modrinth project response: %w", err)
	}
	if p.ID == "" || p.Title == "" {
		return p, errors.New("Modrinth project response is missing ID or title")
	}
	return p, nil
}

func getAPI(c *http.Client, url string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func apiMessage(body []byte) string {
	var e apiError
	if json.Unmarshal(body, &e) == nil {
		if e.Message != "" {
			return e.Message
		}
		if e.Error != "" {
			return e.Error
		}
	}
	return strings.TrimSpace(string(body))
}

func chooseFile(files []modrinthFile, localSHA1 string) (modrinthFile, error) {
	for _, f := range files {
		if strings.EqualFold(f.Hashes["sha1"], localSHA1) && f.URL != "" && f.Filename != "" {
			return f, nil
		}
	}
	for _, f := range files {
		if f.Primary && f.URL != "" && f.Filename != "" && f.Hashes["sha512"] != "" {
			return f, nil
		}
	}
	for _, f := range files {
		if f.URL != "" && f.Filename != "" && f.Hashes["sha512"] != "" {
			return f, nil
		}
	}
	return modrinthFile{}, errors.New("no usable downloadable file with SHA-512 hash in Modrinth response")
}

func makePackwizMeta(p modrinthProject, v modrinthVersion, f modrinthFile, localFilename string) (string, error) {
	if f.URL == "" || f.Hashes["sha512"] == "" {
		return "", errors.New("download URL or SHA-512 hash is missing")
	}
	side := "both"
	if strings.EqualFold(p.ServerSide, "unsupported") {
		side = "client"
	} else if strings.EqualFold(p.ClientSide, "unsupported") {
		side = "server"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "name = %s\nfilename = %s\nside = %s\n\n", strconv.Quote(p.Title), strconv.Quote(localFilename), strconv.Quote(side))
	fmt.Fprintf(&b, "[download]\nurl = %s\nhash-format = \"sha512\"\nhash = %s\n", strconv.Quote(f.URL), strconv.Quote(f.Hashes["sha512"]))
	fmt.Fprintf(&b, "\n[update]\n[update.modrinth]\nmod-id = %s\nversion = %s\n", strconv.Quote(p.ID), strconv.Quote(v.ID))
	return b.String(), nil
}

func sha1File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isNvidiaOnly(projectSlug, configured string) bool {
	for _, slug := range strings.Split(configured, ",") {
		if strings.EqualFold(strings.TrimSpace(slug), projectSlug) && strings.TrimSpace(slug) != "" {
			return true
		}
	}
	return false
}

func safeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func hasProjectID(dir, projectID string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	needle := "mod-id = " + strconv.Quote(projectID)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pw.toml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(string(b), needle) {
			return true
		}
	}
	return false
}

func hasFilename(dir, filename string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	needle := "filename = " + strconv.Quote(filename)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pw.toml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(string(b), needle) {
			return true
		}
	}
	return false
}

func writeExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return nil
}

func moveToProcessed(src, processedDir string) error {
	if err := os.MkdirAll(processedDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(processedDir, filepath.Base(src))
	if _, err := os.Stat(dst); err == nil {
		ext := filepath.Ext(dst)
		base := strings.TrimSuffix(filepath.Base(dst), ext)
		for i := 2; ; i++ {
			candidate := filepath.Join(processedDir, base+"-"+strconv.Itoa(i)+ext)
			if _, statErr := os.Stat(candidate); errors.Is(statErr, os.ErrNotExist) {
				dst = candidate
				break
			}
		}
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-volume fallback: copy first, then remove the source.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	if err = os.Remove(src); err != nil {
		return fmt.Errorf("copied to processed but could not remove original: %w", err)
	}
	return nil
}

func runMrpack(args []string) error {
	fs := flag.NewFlagSet("mrpack", flag.ContinueOnError)
	out := fs.String("out", "mods", "folder for .pw.toml")
	offline := fs.Bool("offline", false, "do not fetch mod titles")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: go run . mrpack [-out mods] pack.mrpack (or legacy: go run . -out mods pack.mrpack)")
	}
	return convertMrpack(fs.Arg(0), *out, *offline)
}

type mrIndex struct {
	Files []struct {
		Path      string            `json:"path"`
		Hashes    map[string]string `json:"hashes"`
		Env       map[string]string `json:"env"`
		Downloads []string          `json:"downloads"`
	} `json:"files"`
	Dependencies map[string]string `json:"dependencies"`
}

var mrURL = regexp.MustCompile(`modrinth\.com/data/([^/]+)/versions/([^/]+)/`)

func extractOverrides(files []*zip.File) error {
	for _, zf := range files {
		rel := ""
		switch {
		case strings.HasPrefix(zf.Name, "overrides/"):
			rel = strings.TrimPrefix(zf.Name, "overrides/")
		case strings.HasPrefix(zf.Name, "client-overrides/"):
			rel = strings.TrimPrefix(zf.Name, "client-overrides/")
		default:
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		rel = filepath.FromSlash(rel)
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("unsafe override path in archive: %q", zf.Name)
		}
		dst := filepath.Clean(rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeOutErr := out.Close()
		closeInErr := rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
	}
	return nil
}

func convertMrpack(packPath, outDir string, offline bool) error {
	r, err := zip.OpenReader(packPath)
	if err != nil {
		return fmt.Errorf("open mrpack: %w", err)
	}
	defer r.Close()
	var idx mrIndex
	foundIndex := false
	for _, zf := range r.File {
		if zf.Name != "modrinth.index.json" {
			continue
		}
		foundIndex = true
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = json.NewDecoder(io.LimitReader(rc, 16<<20)).Decode(&idx)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("decode modrinth.index.json: %w", err)
		}
		break
	}
	if !foundIndex {
		return errors.New("modrinth.index.json not found; this file may not be a valid Modrinth .mrpack")
	}
	if err := extractOverrides(r.File); err != nil {
		return fmt.Errorf("extract overrides: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var unresolved []string
	for _, f := range idx.Files {
		if !strings.HasSuffix(strings.ToLower(f.Path), ".jar") || f.Env["client"] == "unsupported" {
			continue
		}
		side := "both"
		if f.Env["server"] == "unsupported" {
			side = "client"
		}
		var url, modID, verID string
		for _, u := range f.Downloads {
			if m := mrURL.FindStringSubmatch(u); m != nil {
				url, modID, verID = u, m[1], m[2]
				break
			}
		}
		if url == "" && len(f.Downloads) > 0 {
			url = f.Downloads[0]
		}
		if url == "" {
			fmt.Println("MANUAL (no download URL):", f.Path)
			unresolved = append(unresolved, f.Path)
			continue
		}
		hash, hf := f.Hashes["sha512"], "sha512"
		if hash == "" {
			hash, hf = f.Hashes["sha1"], "sha1"
		}
		if hash == "" {
			fmt.Println("MANUAL (no supported hash):", f.Path)
			unresolved = append(unresolved, f.Path)
			continue
		}
		name := filepath.Base(f.Path)
		slug := modID
		if modID != "" && !offline {
			if p, err := fetchProject(client, modID); err == nil {
				if p.Title != "" {
					name = p.Title
				}
				if p.Slug != "" {
					slug = p.Slug
				}
			}
		}
		if slug == "" {
			slug = strings.TrimSuffix(filepath.Base(f.Path), ".jar")
		}
		metaPath := filepath.Join(outDir, safeSlug(slug)+".pw.toml")
		if _, err := os.Stat(metaPath); err == nil {
			metaPath = filepath.Join(outDir, safeSlug(slug)+"-"+strings.TrimSuffix(filepath.Base(f.Path), ".jar")+".pw.toml")
		}
		var b strings.Builder
		fmt.Fprintf(&b, "name = %s\nfilename = %s\nside = %s\n\n", strconv.Quote(name), strconv.Quote(filepath.Base(f.Path)), strconv.Quote(side))
		fmt.Fprintf(&b, "[download]\nurl = %s\nhash-format = %s\nhash = %s\n", strconv.Quote(url), strconv.Quote(hf), strconv.Quote(hash))
		if modID != "" && verID != "" {
			fmt.Fprintf(&b, "\n[update]\n[update.modrinth]\nmod-id = %s\nversion = %s\n", strconv.Quote(modID), strconv.Quote(verID))
		} else {
			fmt.Println("MANUAL (download source is not Modrinth CDN; auto-update unavailable):", f.Path)
		}
		if err := os.WriteFile(metaPath, []byte(b.String()), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", metaPath, err)
		}
		fmt.Println("CREATED:", metaPath)
	}
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return fmt.Errorf("created available metadata, but %d file(s) need manual handling", len(unresolved))
	}
	fmt.Println("Conversion finished. Run `packwiz refresh` and inspect `packwiz list`.")
	return nil
}
