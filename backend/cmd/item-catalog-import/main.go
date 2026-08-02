// Command item-catalog-import installs an operator-supplied Palworld item
// manifest and its icons into Palhelm's data directory. Game art is deliberately
// not distributed with Palhelm.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/8tp/palhelm/internal/itemgrant"
)

const maxIconBytes = 4 << 20

var safeID = regexp.MustCompile(`^[A-Za-z0-9_]{1,160}$`)

var numberedSchematic = regexp.MustCompile(`(?i)\s+Schematic\s+([1-4])$`)

var schematicRarities = map[string]string{
	"1": "Uncommon",
	"2": "Rare",
	"3": "Epic",
	"4": "Legendary",
}

type manifest struct {
	GameVersion string                  `json:"gameVersion"`
	Source      itemgrant.CatalogSource `json:"source"`
	Items       []manifestItem          `json:"items"`
}

type manifestItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Rarity      string `json:"rarity,omitempty"`
	IconURL     string `json:"iconUrl,omitempty"`
	IconPath    string `json:"iconPath,omitempty"`
	MaxQuantity int    `json:"maxQuantity"`
	Grantable   bool   `json:"grantable"`
}

type palmodsEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	HasGameIcon bool   `json:"hasGameIcon"`
	Group       string `json:"group"`
	Canonical   bool   `json:"canonical"`
	IconURL     string `json:"iconUrl"`
}

type catalogDocument struct {
	SchemaVersion int                     `json:"schemaVersion"`
	GameVersion   string                  `json:"gameVersion"`
	Source        itemgrant.CatalogSource `json:"source"`
	Items         []itemgrant.Item        `json:"items"`
}

func main() {
	input := flag.String("manifest", "", "operator-supplied source manifest JSON")
	palmodsHTML := flag.String("palmods-html", "", "locally saved PalMods item reference HTML (alternative to --manifest)")
	gameVersion := flag.String("game-version", "", "exact running Palworld version (required with --palmods-html)")
	outCatalog := flag.String("catalog", "./data/item-catalog.json", "output Palhelm catalogue")
	outIcons := flag.String("icons", "./data/item-icons", "output icon directory")
	outAllowlist := flag.String("allowlist", "", "output bridge allowlist (default: item-allowlist.txt beside catalogue)")
	delay := flag.Duration("request-delay", 750*time.Millisecond, "minimum delay between remote icon requests")
	validateOnly := flag.Bool("validate-only", false, "validate and summarize the source without downloading or writing")
	flag.Parse()
	if (*input == "") == (*palmodsHTML == "") {
		log.Fatal("set exactly one of --manifest or --palmods-html")
	}
	if *delay < 100*time.Millisecond {
		log.Fatal("--request-delay must be at least 100ms")
	}
	if *outAllowlist == "" {
		*outAllowlist = filepath.Join(filepath.Dir(*outCatalog), "item-allowlist.txt")
	}

	var doc manifest
	var err error
	if *palmodsHTML != "" {
		if strings.TrimSpace(*gameVersion) == "" {
			log.Fatal("--game-version is required with --palmods-html")
		}
		doc, err = readPalmodsHTML(*palmodsHTML, *gameVersion)
	} else {
		doc, err = readManifest(*input)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *validateOnly {
		grantable, icons, err := validateManifest(doc)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("validated %d item records for Palworld %s: %d grantable, %d icons", len(doc.Items), doc.GameVersion, grantable, icons)
		return
	}
	if err := os.MkdirAll(*outIcons, 0o700); err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	out := catalogDocument{SchemaVersion: itemgrant.CatalogSchemaVersion, GameVersion: strings.TrimSpace(doc.GameVersion), Source: doc.Source, Items: make([]itemgrant.Item, 0, len(doc.Items))}
	seen := make(map[string]bool, len(doc.Items))
	lastRequest := time.Time{}
	for index, source := range doc.Items {
		item, iconSource, err := prepareItem(source)
		if err != nil {
			log.Fatalf("item %d: %v", index, err)
		}
		key := strings.ToLower(item.ID)
		if seen[key] {
			log.Fatalf("item %d: duplicate id %q", index, item.ID)
		}
		seen[key] = true
		if iconSource != "" {
			ext, extErr := iconExtension(iconSource)
			if extErr != nil {
				log.Fatalf("item %s icon: %v", item.ID, extErr)
			}
			item.Icon = strings.ToLower(item.ID) + ext
			target := filepath.Join(*outIcons, item.Icon)
			if stat, statErr := os.Stat(target); statErr != nil || stat.Size() == 0 {
				if strings.HasPrefix(iconSource, "https://") {
					if wait := *delay - time.Since(lastRequest); wait > 0 {
						time.Sleep(wait)
					}
					err = downloadIcon(client, iconSource, target)
					lastRequest = time.Now()
				} else {
					err = copyIcon(iconSource, target)
				}
				if err != nil {
					log.Fatalf("item %s icon: %v", item.ID, err)
				}
				if err := validateIcon(target, ext); err != nil {
					_ = os.Remove(target)
					log.Fatalf("item %s icon: %v", item.ID, err)
				}
			}
		} else if item.Grantable {
			log.Fatalf("item %s is grantable but has no iconUrl or iconPath", item.ID)
		}
		out.Items = append(out.Items, item)
	}
	if out.GameVersion == "" || strings.TrimSpace(out.Source.Name) == "" {
		log.Fatal("gameVersion and source.name are required")
	}
	if err := writeCatalog(*outCatalog, out); err != nil {
		log.Fatal(err)
	}
	if err := writeAllowlist(*outAllowlist, out); err != nil {
		log.Fatal(err)
	}
	if _, err := itemgrant.LoadCatalog(*outCatalog); err != nil {
		log.Fatalf("post-write validation failed: %v", err)
	}
	log.Printf("installed %d item records for Palworld %s; catalogue=%s icons=%s allowlist=%s", len(out.Items), out.GameVersion, *outCatalog, *outIcons, *outAllowlist)
}

func readPalmodsHTML(path, gameVersion string) (manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return manifest{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if err != nil {
		return manifest{}, err
	}
	if len(b) > 32<<20 {
		return manifest{}, errors.New("PalMods HTML exceeds 32 MiB")
	}
	entries, err := parsePalmodsEntries(string(b))
	if err != nil {
		return manifest{}, err
	}
	if len(entries) < 2000 || len(entries) > 10000 {
		return manifest{}, fmt.Errorf("refusing implausible PalMods item count %d", len(entries))
	}
	doc := manifest{GameVersion: strings.TrimSpace(gameVersion), Source: itemgrant.CatalogSource{Name: "PalMods game-ID reference (operator import)", URL: "https://www.palmods.gg/docs/authors/game-ids/items", GeneratedAt: time.Now().UTC().Format(time.RFC3339)}, Items: make([]manifestItem, 0, len(entries))}
	rarities := palmodsRarities(entries)
	for _, entry := range entries {
		maximum, permittedGroup := palmodsQuantityPolicy(entry.Group)
		grantable := entry.Canonical && entry.HasGameIcon && permittedGroup
		if !grantable {
			maximum = 1
		}
		iconURL := ""
		if entry.HasGameIcon {
			iconURL = entry.IconURL
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" || name == "-" || strings.HasPrefix(name, "PV_") {
			name = entry.ID
		}
		doc.Items = append(doc.Items, manifestItem{ID: entry.ID, Name: name, Category: entry.Group, Rarity: rarities[strings.ToLower(entry.ID)], IconURL: iconURL, MaxQuantity: maximum, Grantable: grantable})
	}
	return doc, nil
}

// palmodsRarities derives only grades that the game IDs and localized names make
// unambiguous. Numbered schematics use Palworld's documented 1–4 quality ladder,
// and Blueprint_<item ID> points at the exact equipment variant it creates. A
// base weapon/armor/glider sharing that variant's localized name is the common
// recipe. Special schematics without a numbered grade remain deliberately blank.
func palmodsRarities(entries []palmodsEntry) map[string]string {
	byID := make(map[string]palmodsEntry, len(entries))
	result := make(map[string]string)
	gradedNames := make(map[string]bool)
	for _, entry := range entries {
		byID[strings.ToLower(entry.ID)] = entry
	}
	for _, entry := range entries {
		if entry.Group != "Schematic" {
			continue
		}
		match := numberedSchematic.FindStringSubmatch(strings.TrimSpace(entry.Name))
		if len(match) != 2 {
			continue
		}
		rarity := schematicRarities[match[1]]
		result[strings.ToLower(entry.ID)] = rarity
		targetID := strings.TrimPrefix(entry.ID, "Blueprint_")
		if target, ok := byID[strings.ToLower(targetID)]; ok {
			result[strings.ToLower(target.ID)] = rarity
			if isQualityEquipment(target.Group) {
				gradedNames[strings.ToLower(strings.TrimSpace(target.Name))] = true
			}
		}
	}
	for _, entry := range entries {
		key := strings.ToLower(entry.ID)
		if _, graded := result[key]; graded || !isQualityEquipment(entry.Group) {
			continue
		}
		if gradedNames[strings.ToLower(strings.TrimSpace(entry.Name))] {
			result[key] = "Common"
		}
	}
	return result
}

func isQualityEquipment(group string) bool {
	switch group {
	case "Weapon", "Armor", "Glider":
		return true
	default:
		return false
	}
}

func validateManifest(doc manifest) (grantable, icons int, err error) {
	if strings.TrimSpace(doc.GameVersion) == "" || strings.TrimSpace(doc.Source.Name) == "" {
		return 0, 0, errors.New("gameVersion and source.name are required")
	}
	seen := make(map[string]bool, len(doc.Items))
	for index, source := range doc.Items {
		item, iconSource, prepareErr := prepareItem(source)
		if prepareErr != nil {
			return 0, 0, fmt.Errorf("item %d: %w", index, prepareErr)
		}
		key := strings.ToLower(item.ID)
		if seen[key] {
			return 0, 0, fmt.Errorf("item %d: duplicate id %q", index, item.ID)
		}
		seen[key] = true
		if iconSource != "" {
			icons++
		}
		if item.Grantable {
			grantable++
			if iconSource == "" {
				return 0, 0, fmt.Errorf("item %s is grantable but has no iconUrl or iconPath", item.ID)
			}
		}
	}
	return grantable, icons, nil
}

func parsePalmodsEntries(body string) ([]palmodsEntry, error) {
	startMarker := `\"entries\":[`
	endMarker := `}]},\"categories\"`
	start := strings.Index(body, startMarker)
	if start < 0 {
		return nil, errors.New("PalMods item entries were not found; the page format may have changed")
	}
	start += len(startMarker) - 1
	endRelative := strings.Index(body[start:], endMarker)
	if endRelative < 0 {
		return nil, errors.New("PalMods item entry terminator was not found; the page format may have changed")
	}
	escaped := body[start : start+endRelative+2]
	unescaped, err := strconv.Unquote(`"` + escaped + `"`)
	if err != nil {
		return nil, fmt.Errorf("decode PalMods page payload: %w", err)
	}
	var entries []palmodsEntry
	if err := json.Unmarshal([]byte(unescaped), &entries); err != nil {
		return nil, fmt.Errorf("decode PalMods entries: %w", err)
	}
	return entries, nil
}

func palmodsQuantityPolicy(group string) (int, bool) {
	switch group {
	case "Material", "Ore & Gem", "Pal Drop":
		return 9999, true
	case "Ammo", "Food", "Medicine", "Sphere", "Seed", "Consumable":
		return 999, true
	case "Weapon", "Accessory", "Armor", "Glider", "Pal Gear", "Schematic":
		return 10, true
	default:
		// Key Item and Internal & other are intentionally denylisted.
		return 1, false
	}
}

func readManifest(path string) (manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return manifest{}, err
	}
	defer f.Close()
	var doc manifest
	dec := json.NewDecoder(io.LimitReader(f, 32<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if len(doc.Items) < 1 || len(doc.Items) > 10000 {
		return manifest{}, errors.New("manifest must contain 1 to 10000 items")
	}
	return doc, nil
}

func prepareItem(source manifestItem) (itemgrant.Item, string, error) {
	source.ID, source.Name = strings.TrimSpace(source.ID), strings.TrimSpace(source.Name)
	if !safeID.MatchString(source.ID) {
		return itemgrant.Item{}, "", fmt.Errorf("invalid id %q", source.ID)
	}
	if source.Name == "" || len(source.Name) > 160 {
		return itemgrant.Item{}, "", errors.New("name is required and must not exceed 160 characters")
	}
	if source.MaxQuantity < 1 || source.MaxQuantity > 9999 {
		return itemgrant.Item{}, "", errors.New("maxQuantity must be from 1 to 9999")
	}
	if source.IconURL != "" && source.IconPath != "" {
		return itemgrant.Item{}, "", errors.New("set only one of iconUrl and iconPath")
	}
	iconSource := strings.TrimSpace(source.IconURL)
	if iconSource != "" {
		u, err := url.Parse(iconSource)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return itemgrant.Item{}, "", errors.New("iconUrl must be an absolute HTTPS URL without credentials")
		}
	} else if source.IconPath != "" {
		absolute, err := filepath.Abs(source.IconPath)
		if err != nil {
			return itemgrant.Item{}, "", err
		}
		iconSource = absolute
	}
	return itemgrant.Item{ID: source.ID, Name: source.Name, Description: strings.TrimSpace(source.Description), Category: strings.TrimSpace(source.Category), Rarity: strings.TrimSpace(source.Rarity), MaxQuantity: source.MaxQuantity, Grantable: source.Grantable}, iconSource, nil
}

func iconExtension(source string) (string, error) {
	ext := strings.ToLower(filepath.Ext(strings.Split(source, "?")[0]))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return ext, nil
	}
	return "", errors.New("icon source must end in .png, .jpg, .jpeg, or .webp")
}

func validateIcon(path, extension string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	header = header[:n]
	valid := false
	switch extension {
	case ".png":
		valid = len(header) >= 8 && string(header[:8]) == "\x89PNG\r\n\x1a\n"
	case ".jpg", ".jpeg":
		valid = len(header) >= 3 && header[0] == 0xff && header[1] == 0xd8 && header[2] == 0xff
	case ".webp":
		valid = len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP"
	}
	if !valid {
		return fmt.Errorf("file content does not match %s", extension)
	}
	return nil
}

func downloadIcon(client *http.Client, source, target string) error {
	req, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "PalhelmItemCatalogImporter/1.0 (+https://github.com/8tp/palhelm)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("source returned %q, not an image", contentType)
	}
	return writeAtomic(target, io.LimitReader(resp.Body, maxIconBytes+1), true)
}

func copyIcon(source, target string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeAtomic(target, io.LimitReader(f, maxIconBytes+1), true)
}

func writeAtomic(target string, source io.Reader, enforceSize bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".item-icon-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	n, copyErr := io.Copy(tmp, source)
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n == 0 || (enforceSize && n > maxIconBytes) {
		return fmt.Errorf("icon size %d is invalid", n)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func writeCatalog(target string, doc catalogDocument) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(target, strings.NewReader(string(append(b, '\n'))), false)
}

func writeAllowlist(target string, doc catalogDocument) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString("# palhelm-item-allowlist-v1\n")
	body.WriteString("# gameVersion=" + doc.GameVersion + "\n")
	count := 0
	for _, item := range doc.Items {
		if item.Grantable {
			count++
		}
	}
	body.WriteString(fmt.Sprintf("# catalogItems=%d\n", count))
	for _, item := range doc.Items {
		if item.Grantable {
			body.WriteString(fmt.Sprintf("%s\t%d\n", item.ID, item.MaxQuantity))
		}
	}
	return writeAtomic(target, strings.NewReader(body.String()), false)
}

func init() {
	// Register common image types on minimal systems where mime's OS database is absent.
	_ = mime.AddExtensionType(".webp", "image/webp")
}
