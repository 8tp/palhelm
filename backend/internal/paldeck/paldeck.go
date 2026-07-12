// Package paldeck maps Palworld's internal save-file CharacterID identifiers to their
// current in-game display names.
//
// The save format only ever stores a CharacterID (e.g. "SheepBall"), never the display name a
// player sees in-game ("Lamball"). Palhelm previously surfaced that raw internal ID directly
// wherever a Pal's name appeared in the API — this package is the fix.
//
// # Data provenance
//
// Facts (a game's internal identifiers and the display names they correspond to) are not
// copyrightable; the pairs below are reproduced from community reference data, cited per
// section. Any pal whose 1.0-era internal CharacterID could not be confirmed from a source is
// intentionally left out — guessing a plausible-looking ID would silently mislabel an
// unrelated Pal, which is worse than the raw-id fallback Lookup/Name already provide.
//
// # Regenerating this table
//
// Each map is one entry per line, `"internalIDLowercase": "Display Name",`, sorted by key, and
// grouped by data-drop/source so a future refresh (e.g. once Pocketpair or a community project
// publishes an authoritative post-1.0 Paldeck export) can replace a whole map body rather than
// hand-editing individual lines. Keys are lowercased because save data does not consistently
// preserve the internal ID's original casing.
package paldeck

import (
	"regexp"
	"sort"
	"strings"
)

// legacyNames covers the pre-1.0 roster (~156 base species plus their regional/elemental
// variants).
//
// Source: https://palworld.wiki.gg/wiki/Pals_/_Internal_names (fetched 2026-07-10). The first
// several entries (Lamball/SheepBall, Cattiva/PinkCat, Chikipi/ChickenPal, Lifmunk/Carbunclo,
// Foxparks/Kitsunebi) were cross-checked against the community "All Pals' Internal File Names"
// Steam guide (steamcommunity.com/sharedfiles/filedetails/?id=3596582470) and matched exactly.
// Alpaca→Melpaca and PlantSlime_Flower→Gumoss were corrected from PalCalc v1.17.2's pinned 1.0
// export after a read-only comparison against the live roster on 2026-07-11.
var legacyNames = map[string]string{
	"alpaca":                  "Melpaca",
	"amaterasuwolf":           "Kitsun",
	"anubis":                  "Anubis",
	"badcatgirl":              "Nyafia",
	"baphomet":                "Incineram",
	"baphomet_dark":           "Incineram Noct",
	"bastet":                  "Mau",
	"bastet_ice":              "Mau Cryst",
	"berrygoat":               "Caprity",
	"birddragon":              "Vanwyrm",
	"birddragon_ice":          "Vanwyrm Cryst",
	"blackcentaur":            "Necromus",
	"blackfurdragon":          "Dragostrophe",
	"blackgriffon":            "Shadowbeak",
	"blackmetaldragon":        "Astegon",
	"blueberryfairy":          "Prunelia",
	"bluedragon":              "Azurobe",
	"blueplatypus":            "Fuack",
	"boar":                    "Rushoar",
	"candleghost":             "Sootseer",
	"captainpenguin":          "Penking",
	"carbunclo":               "Lifmunk",
	"catbat":                  "Tombat",
	"catmage":                 "Katress",
	"catmage_fire":            "Katress Ignis",
	"catvampire":              "Felbat",
	"chickenpal":              "Chikipi",
	"colorfulbird":            "Tocotoco",
	"cowpal":                  "Mozzarina",
	"cutebutterfly":           "Cinnamoth",
	"cutefox":                 "Vixy",
	"cutemole":                "Fuddler",
	"darkalien":               "Xenovader",
	"darkcrow":                "Cawgnito",
	"darkmutant":              "Dark Mutant",
	"darkscorpion":            "Menasting",
	"darkscorpion_ground":     "Menasting Terra",
	"deer":                    "Eikthyrdeer",
	"deer_ground":             "Eikthyrdeer Terra",
	"dreamdemon":              "Daedream",
	"drillgame":               "Digtoise",
	"eagle":                   "Galeclaw",
	"eleccat":                 "Sparkit",
	"eleclion":                "Boltmane",
	"elecpanda":               "Grizzbolt",
	"fairydragon":             "Elphidran",
	"fairydragon_water":       "Elphidran Aqua",
	"featherostrich":          "Dazemu",
	"fengyundeeper":           "Fenglope",
	"firekirin":               "Pyrin",
	"firekirin_dark":          "Pyrin Noct",
	"flamebambi":              "Rooby",
	"flamebuffalo":            "Arsox",
	"flowerdinosaur":          "Dinossom",
	"flowerdinosaur_electric": "Dinossom Lux",
	"flowerdoll":              "Petallia",
	"flowerrabbit":            "Flopie",
	"flyingmanta":             "Celaray",
	"foxmage":                 "Wixen",
	"foxmage_dark":            "Wixen Noct",
	"ganesha":                 "Teafant",
	"garm":                    "Direhowl",
	"ghostbeast":              "Maraith",
	"gorilla":                 "Gorirat",
	"gorilla_ground":          "Gorirat Terra",
	"grassmammoth":            "Mammorest",
	"grassmammoth_ice":        "Mammorest Cryst",
	"grasspanda":              "Mossanda",
	"grasspanda_electric":     "Mossanda Lux",
	"grassrabbitman":          "Verdash",
	"guardiandog":             "Yakumo",
	"hadesbird":               "Helzephyr",
	"hadesbird_electric":      "Helzephyr Lux",
	"hawkbird":                "Nitewing",
	"hedgehog":                "Jolthog",
	"hedgehog_ice":            "Jolthog Cryst",
	"herculesbeetle":          "Warsect",
	"herculesbeetle_ground":   "Warsect Terra",
	"horus":                   "Faleris",
	"icedeer":                 "Reindrix",
	"icefox":                  "Foxcicle",
	"icehorse":                "Frostallion",
	"icehorse_dark":           "Frostallion Noct",
	"jetdragon":               "Jetragon",
	"kelpie":                  "Kelpsea",
	"kelpie_fire":             "Kelpsea Ignis",
	"kendofrog":               "Croajiro",
	"kingalpaca":              "Kingpaca",
	"kingalpaca_ice":          "Kingpaca Cryst",
	"kingbahamut":             "Blazamut",
	"kingbahamut_dragon":      "Blazamut Ryu",
	"kirin":                   "Univolt",
	"kitsunebi":               "Foxparks",
	"lavagirl":                "Flambelle",
	"lazycatfish":             "Dumud",
	"lazydragon":              "Relaxaurus",
	"lazydragon_electric":     "Relaxaurus Lux",
	"leafprincess":            "Lullu",
	"lilyqueen":               "Lyleen",
	"lilyqueen_dark":          "Lyleen Noct",
	"littlebriarrose":         "Bristla",
	"lizardman":               "Leezpunk",
	"lizardman_fire":          "Leezpunk Ignis",
	"manticore":               "Blazehowl",
	"manticore_dark":          "Blazehowl Noct",
	"mimicdog":                "Mimog",
	"monkey":                  "Tanzee",
	"moonqueen":               "Selyne",
	"mopbaby":                 "Swee",
	"mopking":                 "Sweepa",
	"mushroomdragon":          "Shroomer",
	"mushroomdragon_dark":     "Shroomer Noct",
	"mutant":                  "Lunaris",
	"naughtycat":              "Grintale",
	"negativekoala":           "Depresso",
	"negativeoctopus":         "Killamari",
	"nightbluehorse":          "Starryon",
	"nightfox":                "Nox",
	"nightlady":               "Bellanoir",
	"nightlady_dark":          "Bellanoir Libero",
	"penguin":                 "Pengullet",
	"pinkcat":                 "Cattiva",
	"pinklizard":              "Lovander",
	"pinkrabbit":              "Ribbuny",
	"plantslime":              "Gumoss",
	"plantslime_flower":       "Gumoss",
	"queenbee":                "Elizabee",
	"raijindaughter":          "Dazzi",
	"redarmorbird":            "Ragnahawk",
	"robinhood":               "Robinquill",
	"robinhood_ground":        "Robinquill Terra",
	"ronin":                   "Bushi",
	"ronin_dark":              "Bushi Noct",
	"saintcentaur":            "Paladius",
	"sakurasaurus":            "Broncherry",
	"sakurasaurus_water":      "Broncherry Aqua",
	"scorpionman":             "Prixter",
	"serpent":                 "Surfent",
	"serpent_ground":          "Surfent Terra",
	"sharkkid":                "Gobfin",
	"sharkkid_fire":           "Gobfin Ignis",
	"sheepball":               "Lamball",
	"sifudog":                 "Dogen",
	"skydragon":               "Quivern",
	"skydragon_grass":         "Quivern Botan",
	"smallarmadillo":          "Kikit",
	"soldierbee":              "Beegarde",
	"suzaku":                  "Suzaku",
	"suzaku_water":            "Suzaku Aqua",
	"sweetssheep":             "Woolipop",
	"thunderbird":             "Beakon",
	"thunderdog":              "Rayhound",
	"thunderdragonman":        "Orserk",
	"umihebi":                 "Jormuntide",
	"umihebi_fire":            "Jormuntide Ignis",
	"violetfairy":             "Vaelet",
	"volcanicmonster":         "Reptyro",
	"volcanicmonster_ice":     "Reptyro Cryst",
	"weaseldragon":            "Chillet",
	"weaseldragon_fire":       "Chillet Ignis",
	"werewolf":                "Loupmoon",
	"whitealiendragon":        "Xenogard",
	"whitedeer":               "Celesdir",
	"whitemoth":               "Sibelyx",
	"whiteshielddragon":       "Silvegis",
	"whitetiger":              "Cryolinx",
	"windchimes":              "Hangyu",
	"windchimes_ice":          "Hangyu Cryst",
	"winggolem":               "Knocklem",
	"wizardowl":               "Hoocrates",
	"woolfox":                 "Cremis",
	"yeti":                    "Wumpo",
	"yeti_grass":              "Wumpo Botan",
}

// v1Names covers the 72 Palworld 1.0 additions catalogued in
// docs/research/raw/content-audit-codex.md §3, plus three 1.0 IDs subsequently observed in the
// live save audit (Herbil, Ribbuny Botan, and Tarantriss). A couple of "Lux" variants resolve to
// a suffix (_Thunder) that doesn't match the pre-1.0 "Lux = _Electric" convention seen in
// legacyNames above — Pocketpair's own internal naming isn't fully consistent.
//
// Sources: https://paldb.cc (each pal's own "Code" field), fetched 2026-07-10; the three live
// additions were cross-checked against PalCalc v1.17.2 and Palworld Save Pal's English
// localization export, both pinned in bot/data/pal-knowledge.json.
var v1Names = map[string]string{
	"blackpuppy_ice":         "Smokie Cryst",
	"blueskydragon":          "Shaolong",
	"brownrabbit":            "Lapiron",
	"cactusdoll":             "Needoll",
	"cactusdoll_dark":        "Needoll Noct",
	"clionetwins":            "Amione",
	"cloverfairy":            "Clovee",
	"clownrabbit":            "Dupin",
	"cubeturtle":             "Tetroise",
	"cubeturtle_neutral":     "Tetroise Primo",
	"dandeliongirl":          "Souffline",
	"darkflamefox":           "Majex",
	"domearmordragon":        "Aegidron",
	"eleclizard":             "Slowatt",
	"elecpomeranian":         "Puffolt",
	"elecsnail":              "Snock",
	"elecsnail_ground":       "Snock Lux",
	"flowerdoll_fire":        "Petallia Ignis",
	"flowerprince":           "Dandilord",
	"fluffybird":             "Muffly",
	"foxexorcist":            "Flaracle",
	"ghostblackcat":          "Wispaw",
	"ghostdragon":            "Eidrolon",
	"ghostdragon_fire":       "Eidrolon Ignis",
	"ghostrabbit_grass":      "Nitemary Botan",
	"grassgolem":             "Dualith",
	"grassgolem_dark":        "Dualith Noct",
	"grassminotaur":          "Elgrove",
	"grassminotaur_ice":      "Elgrove Cryst",
	"hoodghost":              "Hoodle",
	"iceseal_ground":         "Polapup Terra",
	"kabukiman":              "Renjishi",
	"kingsunfish":            "Solmora",
	"kingsunfish_thunder":    "Solmora Lux",
	"kingwhale":              "Panthalus",
	"kirin_ice":              "Univolt Cryst",
	"kitsunebi_ice":          "Foxparks Cryst",
	"lanternbutler":          "Loomen",
	"leafmomonga":            "Herbil",
	"longcat":                "Valentail",
	"lotusdragon":            "Ophydia",
	"monkey_fire":            "Tanzee Ignis",
	"monochromequeen":        "Solenne",
	"moonchild":              "Wistella",
	"mothman":                "Silvance",
	"mummypal":               "Gildra",
	"mushroomlady":           "Mycora",
	"nightbluehorse_neutral": "Starryon Primo",
	"octopusgirl_neutral":    "Gloopie Primo",
	"onighostgirl":           "Bakemi",
	"pandagirl":              "Leafan",
	"pinkrabbit_grass":       "Ribbuny Botan",
	"purplespider":           "Tarantriss",
	"redflowerbird":          "Tropicaw",
	"rockbeast":              "Pierdon",
	"rockbeast_ice":          "Pierdon Cryst",
	"samuraidog":             "Pupperai",
	"scorpionman_electric":   "Prixter Lux",
	"sekhmet":                "Sekhmet",
	"sleeverabbit":           "Lapure",
	"smallyeti":              "Snugloo",
	"snakegirl":              "Venusa",
	"sumodog":                "Bulldosu",
	"sweetssheep_ground":     "Woolipop Terra",
	"swordcutlassfish":       "Skutlass",
	"swordcutlassfish_fire":  "Skutlass Ignis",
	"thiefbird":              "Roujay",
	"thunderbird_ice":        "Beakon Cryst",
	"thunderdog_ice":         "Rayhound Cryst",
	"thunderfluffybird":      "Dynamoff",
	"venusflytrap":           "Carnibora",
	"volcanodragon":          "Moldron",
	"volcanodragon_ice":      "Moldron Cryst",
	"whitedeer_dark":         "Celesdir Noct",
	"whitemoth_neutral":      "Sibelyx Primo",
	"winggolem_fire":         "Knocklem Ignis",
}

// exactNames contains the small number of prefixed save IDs whose prefix is part of the
// character's identity, rather than merely an Alpha/boss marker. In particular,
// BOSS_Hunter_Rifle is the named bounty target Hawk; stripping BOSS_ first and resolving the
// ordinary Hunter_Rifle NPC would incorrectly label Hawk as a Syndicate Gunner.
//
// Source: Palworld Save Pal's pinned English 1.0 localization export at commit
// e46188978a13e74d84c9a1ce5569497ee0555cae, cross-checked against paldb.cc's NPC table on
// 2026-07-11.
var exactNames = map[string]string{
	"boss_hunter_rifle": "Hawk",
}

// humanNames covers capturable human NPCs that have appeared in the same save collection as
// Pals. Keeping these separately documents that they are not Pal species while still preventing
// internal weapon/archetype IDs from leaking into public API display names.
//
// Source: the same pinned Palworld Save Pal English localization export cited above.
var humanNames = map[string]string{
	"hunter_rifle": "Syndicate Gunner",
}

// Lookup returns the display name mapped to a save-file CharacterID, case-insensitively (save
// data does not reliably preserve the internal ID's original casing). ok is false when id is
// not present in either table.
func Lookup(id string) (string, bool) {
	rawKey := strings.ToLower(strings.TrimSpace(id))
	if name, ok := exactNames[rawKey]; ok {
		return name, true
	}
	key := strings.ToLower(BaseCharacterID(id))
	if name, ok := v1Names[key]; ok {
		return name, true
	}
	if name, ok := humanNames[key]; ok {
		return name, true
	}
	name, ok := legacyNames[key]
	return name, ok
}

// Name returns the mapped display name for id, or id itself unchanged when unmapped — the
// historical Palhelm behavior, kept as a safe fallback so an unrecognized Pal still shows
// something rather than an empty name.
func Name(id string) string {
	if name, ok := Lookup(id); ok {
		return name
	}
	// Generic, unnamed human NPCs (e.g. BOSS_Female_People03) have no unique
	// in-game name, so labeling the archetype is accurate rather than a guess.
	if name, ok := genericHumanName(id); ok {
		return name
	}
	// Boss captures use BOSS_<species ID> in CharacterSaveParameterMap. The
	// prefix describes the instance variant, not the species name; callers show
	// that separately through isAlpha/a boss emblem.
	trimmed := BaseCharacterID(id)
	if trimmed != "" {
		return trimmed
	}
	return id
}

// genericHumanRe matches Palworld's anonymous filler-human NPCs: an optional
// gender, the "People" tribe token, and a numeric variant, e.g. Female_People03.
var genericHumanRe = regexp.MustCompile(`^(?:(male|female)_)?people\d*$`)

// genericHumanName resolves only the anonymous "People" human NPCs to a readable
// archetype label. It deliberately does not touch named human bosses (Hawk, tower
// bosses) — those must stay in the curated tables — nor any Pal species.
func genericHumanName(id string) (string, bool) {
	match := genericHumanRe.FindStringSubmatch(strings.ToLower(BaseCharacterID(id)))
	if match == nil {
		return "", false
	}
	switch match[1] {
	case "male":
		return "Human (Male)", true
	case "female":
		return "Human (Female)", true
	default:
		return "Human", true
	}
}

// BaseCharacterID removes save prefixes that describe an instance variant, not
// a Pal species. The original CharacterID remains available in stored/API data.
func BaseCharacterID(id string) string {
	value := strings.TrimSpace(id)
	for strings.HasPrefix(strings.ToLower(value), "boss_") {
		value = value[len("boss_"):]
	}
	return value
}

// IsBossID recognizes captured boss variants even when Palworld omits/clears
// the separate IsBoss SaveParameter flag.
func IsBossID(id string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), "boss_")
}

// Entry pairs a lowercased CharacterID with its resolved display name.
type Entry struct {
	ID   string
	Name string
}

// All returns every known CharacterID→display-name pair, sorted by ID. It exists so tooling
// external to this package (namely cmd/paldeck-list, which backs scripts/fetch-pal-icons.sh) can
// enumerate the roster without a second, hand-maintained copy of it drifting out of sync with the
// tables above. v1Names wins on key collisions, matching Lookup's precedence.
func All() []Entry {
	merged := make(map[string]string, len(legacyNames)+len(v1Names)+len(humanNames)+len(exactNames))
	for id, name := range legacyNames {
		merged[id] = name
	}
	for id, name := range v1Names {
		merged[id] = name
	}
	for id, name := range humanNames {
		merged[id] = name
	}
	for id, name := range exactNames {
		merged[id] = name
	}
	out := make([]Entry, 0, len(merged))
	for id, name := range merged {
		out = append(out, Entry{ID: id, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
