// Package gameconfig provides a compose-environment-backed Palworld settings editor.
package gameconfig

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ValueType describes API validation and UI controls.
type ValueType string

const (
	String  ValueType = "string"
	Integer ValueType = "integer"
	Number  ValueType = "number"
	Boolean ValueType = "boolean"
)

// CatalogEntry maps an image environment variable to PalWorldSettings.ini.
type CatalogEntry struct {
	Env, INI         string
	Type             ValueType
	Group            string
	Default          any
	Masked, ReadOnly bool
}

var catalog = []CatalogEntry{
	{"SERVER_NAME", "ServerName", String, "general", "Palworld Server", false, false},
	{"SERVER_DESCRIPTION", "ServerDescription", String, "general", "", false, false},
	{"SERVER_PASSWORD", "ServerPassword", String, "general", "", true, false},
	{"ADMIN_PASSWORD", "AdminPassword", String, "general", "", true, false},
	{"PLAYERS", "ServerPlayerMaxNum", Integer, "general", 32, false, false},
	{"PORT", "PublicPort", Integer, "network", 8211, false, false},
	{"PUBLIC_IP", "PublicIP", String, "network", "", false, false},
	{"PUBLIC_PORT", "PublicPort", Integer, "network", 8211, false, false},
	{"MULTITHREADING", "bUseMultithreadForDS", Boolean, "network", true, false, false},
	{"COMMUNITY_SERVER", "CommunityServer", Boolean, "network", false, false, false},
	{"EXP_RATE", "ExpRate", Number, "gameplay", 1.0, false, false},
	{"PAL_CAPTURE_RATE", "PalCaptureRate", Number, "gameplay", 1.0, false, false},
	{"DAY_TIME_SPEED_RATE", "DayTimeSpeedRate", Number, "gameplay", 1.0, false, false},
	{"NIGHT_TIME_SPEED_RATE", "NightTimeSpeedRate", Number, "gameplay", 1.0, false, false},
	{"PAL_SPAWN_NUM_RATE", "PalSpawnNumRate", Number, "gameplay", 1.0, false, false},
	{"DEATH_PENALTY", "DeathPenalty", String, "gameplay", "All", false, false},
	{"DIFFICULTY", "Difficulty", String, "gameplay", "None", false, false},
	{"RCON_ENABLED", "RCONEnabled", Boolean, "panel-managed", true, false, true},
	{"RCON_PORT", "RCONPort", Integer, "panel-managed", 25575, false, true},
	{"REST_API_ENABLED", "RESTAPIEnabled", Boolean, "panel-managed", true, false, true},
	{"REST_API_PORT", "RESTAPIPort", Integer, "panel-managed", 8212, false, true},

	// --- Palworld 1.0 additions ---
	// Env names verified against the thijsvanloef/palworld-server-docker README (the image
	// that regenerates PalWorldSettings.ini from these vars); ini names + defaults verified
	// against docs.palworldgame.com's post-1.0 configuration reference.
	{"ENABLE_VOICE_CHAT", "bEnableVoiceChat", Boolean, "voice", false, false, false},
	{"VOICE_CHAT_MAX_VOLUME_DISTANCE", "VoiceChatMaxVolumeDistance", Number, "voice", 3000.0, false, false},
	// VoiceChatZeroVolumeDistance is the distance at which voice fades to silence; it only
	// makes sense as a value >= VoiceChatMaxVolumeDistance (the "at full volume" distance).
	// Cross-field validated in Update, see validateVoiceChatDistances.
	{"VOICE_CHAT_ZERO_VOLUME_DISTANCE", "VoiceChatZeroVolumeDistance", Number, "voice", 15000.0, false, false},
	// Privacy: when enabled, buildings display the placing player's ID to everyone on the
	// server, which deanonymizes who built what. Defaults off for that reason.
	{"ENABLE_BUILDING_PLAYER_UID_DISPLAY", "bEnableBuildingPlayerUIdDisplay", Boolean, "gameplay", false, false, false},
	{"MONSTER_FARM_ACTION_SPEED_RATE", "MonsterFarmActionSpeedRate", Number, "gameplay", 1.0, false, false},
	// -1 means unlimited: all physics-active dropped items stay simulated regardless of count.
	// Any value >= 0 caps the number of drops that get full physics simulation (older/excess
	// drops fall back to a cheaper resting state) as a performance knob for large servers.
	{"PHYSICS_ACTIVE_DROP_ITEM_MAX_NUM", "PhysicsActiveDropItemMaxNum", Integer, "gameplay", -1, false, false},
	// Advanced/perf-tuning knob, not documented as a stable public setting yet; expose under
	// "advanced" rather than alongside the everyday gameplay rates.
	{"BUILDING_NAME_DISPLAY_CACHE_TTL_SECONDS", "BuildingNameDisplayCacheTTLSeconds", Integer, "advanced", 60, false, false},
}

// Catalog returns a copy of the supported settings table.
func Catalog() []CatalogEntry { return append([]CatalogEntry(nil), catalog...) }
func byEnv(key string) (CatalogEntry, bool) {
	for _, c := range catalog {
		if c.Env == key {
			return c, true
		}
	}
	return CatalogEntry{}, false
}

// Setting is the merged desired/effective representation.
type Setting struct {
	Key                   string `json:"key"`
	Value, EffectiveValue any
	Type                  ValueType `json:"type"`
	Group                 string    `json:"group"`
	Default               any       `json:"default"`
	Pending               bool      `json:"pending"`
	Editable              bool      `json:"editable"`
	ReadOnly              bool      `json:"readOnly"`
}

func (s Setting) MarshalJSON() ([]byte, error) { // explicit tags for the interface-valued fields
	return []byte(fmt.Sprintf(`{"key":%s,"value":%s,"effectiveValue":%s,"type":%s,"group":%s,"default":%s,"pending":%t,"editable":%t,"readOnly":%t}`, quoteJSON(s.Key), jsonValue(s.Value), jsonValue(s.EffectiveValue), quoteJSON(string(s.Type)), quoteJSON(s.Group), jsonValue(s.Default), s.Pending, s.Editable, s.ReadOnly)), nil
}
func quoteJSON(s string) string { return strconv.Quote(s) }
func jsonValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return strconv.Quote(fmt.Sprint(x))
	}
}

// Capability describes whether an operation is safe in the detected deployment.
type Capability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Capabilities reports the compose write and apply state detected by the backend.
type Capabilities struct {
	Write Capability `json:"write"`
	Apply Capability `json:"apply"`
}

// Response is returned by GET and PUT /config.
type Response struct {
	Source        string       `json:"source"`
	ComposeFile   string       `json:"composeFile,omitempty"`
	Service       string       `json:"service"`
	Version       string       `json:"version,omitempty"`
	Capabilities  Capabilities `json:"capabilities"`
	ManualCommand string       `json:"manualCommand"`
	Settings      []Setting    `json:"settings"`
}

var (
	// ErrReadOnly means the compose deployment does not support safe atomic writes.
	ErrReadOnly = errors.New("configuration is read-only")
	// ErrConflict means the compose file changed after the client read it.
	ErrConflict = errors.New("compose file changed externally")
	// ErrApplyDisabled means one-click apply is intentionally unavailable in v0.3.0.
	ErrApplyDisabled = errors.New("one-click Docker apply is disabled")
)

// Editor combines compose, REST, and ini sources.
type Editor struct {
	ComposeFile, Service, SaveDir string
	DockerControl                 bool
	Effective                     func(context.Context) (map[string]any, error)

	mu         sync.Mutex
	probed     bool
	writeState Capability
}

// Probe detects whether the configured compose file can be replaced atomically. The result is
// retained for the process lifetime so a deployment cannot silently change from read-only to
// writable without an operator restart.
func (e *Editor) Probe() Capability {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.probeLocked()
}

func (e *Editor) probeLocked() Capability {
	if e.probed {
		return e.writeState
	}
	e.probed = true
	state := Capability{Available: false}
	defer func() { e.writeState = state }()
	if e.ComposeFile == "" {
		state.Reason = "PALHELM_COMPOSE_FILE is not configured"
		return state
	}
	info, err := os.Stat(e.ComposeFile)
	if err != nil {
		state.Reason = fmt.Sprintf("compose file is unavailable: %v", err)
		return state
	}
	if !info.Mode().IsRegular() {
		state.Reason = "compose path is not a regular file"
		return state
	}
	if mountedFile(e.ComposeFile) {
		state.Reason = "compose file is mounted as a single file; mount its containing directory read-write instead"
		return state
	}
	dir := filepath.Dir(e.ComposeFile)
	tmp, err := os.CreateTemp(dir, ".palhelm-capability-*")
	if err != nil {
		state.Reason = fmt.Sprintf("compose directory is not writable: %v", err)
		return state
	}
	first := tmp.Name()
	second := first + ".rename"
	_ = tmp.Close()
	defer os.Remove(first)
	defer os.Remove(second)
	if err := os.Rename(first, second); err != nil {
		state.Reason = fmt.Sprintf("compose directory does not support atomic rename: %v", err)
		return state
	}
	state.Available = true
	return state
}

func mountedFile(path string) bool {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	want, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 5 && unescapeMount(fields[4]) == want {
			return true
		}
	}
	return false
}

func unescapeMount(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}

func (e *Editor) iniPath() string {
	return filepath.Join(e.SaveDir, "Config", "LinuxServer", "PalWorldSettings.ini")
}

// Raw returns the INI unchanged.
func (e *Editor) Raw() ([]byte, error) { return os.ReadFile(e.iniPath()) }

// Get returns all catalog settings. REST is preferred over INI for effective values.
func (e *Editor) Get(ctx context.Context) (Response, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	writeState := e.probeLocked()
	desired := map[string]string{}
	source := "ini"
	version := ""
	if e.ComposeFile != "" {
		b, err := os.ReadFile(e.ComposeFile)
		if err != nil {
			if writeState.Available {
				writeState = Capability{Reason: fmt.Sprintf("compose file cannot be read: %v", err)}
			}
		} else {
			desired, _, err = parseEnvironment(string(b), e.Service)
			if err != nil {
				if writeState.Available {
					writeState = Capability{Reason: fmt.Sprintf("compose file cannot be edited safely: %v", err)}
				}
			} else {
				source = "compose"
				version = fileVersion(b)
			}
		}
	}
	ini := map[string]string{}
	if b, err := e.Raw(); err == nil {
		ini = parseINI(string(b))
	} else if !os.IsNotExist(err) {
		return Response{}, err
	}
	effective := map[string]any{}
	restOK := false
	if e.Effective != nil {
		if v, err := e.Effective(ctx); err == nil {
			effective = v
			restOK = true
		}
	}
	settings := make([]Setting, 0, len(catalog))
	for _, c := range catalog {
		dv, has := desired[c.Env]
		var desiredValue any = c.Default
		if has {
			desiredValue = parseTyped(dv, c.Type)
		}
		var ev any
		found := false
		if restOK {
			ev, found = lookupFold(effective, c.INI)
			if !found {
				ev, found = lookupFold(effective, c.Env)
			}
		}
		if !found {
			if v, ok := ini[c.INI]; ok {
				ev = parseTyped(v, c.Type)
				found = true
			}
		}
		if !found {
			ev = c.Default
		}
		pending := fmt.Sprint(desiredValue) != fmt.Sprint(ev)
		if source == "ini" {
			desiredValue = ev
			pending = false
		}
		if c.Masked {
			desiredValue = "•••"
			ev = "•••"
		}
		editable := source == "compose" && writeState.Available && !c.ReadOnly
		settings = append(settings, Setting{Key: c.Env, Value: desiredValue, EffectiveValue: ev, Type: c.Type, Group: c.Group, Default: c.Default, Pending: pending, Editable: editable, ReadOnly: !editable})
	}
	return e.response(source, version, writeState, settings), nil
}

func (e *Editor) response(source, version string, write Capability, settings []Setting) Response {
	return Response{
		Source:      source,
		ComposeFile: e.ComposeFile,
		Service:     e.Service,
		Version:     version,
		Capabilities: Capabilities{
			Write: write,
			Apply: Capability{Reason: "one-click Docker apply is intentionally disabled; run Compose on the host to preserve project identity and host paths"},
		},
		ManualCommand: "docker compose up -d " + e.Service,
		Settings:      settings,
	}
}

func fileVersion(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }

func lookupFold(m map[string]any, key string) (any, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}
func parseTyped(v string, t ValueType) any {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	switch t {
	case Integer:
		n, e := strconv.ParseInt(v, 10, 64)
		if e == nil {
			return n
		}
	case Number:
		n, e := strconv.ParseFloat(v, 64)
		if e == nil {
			return n
		}
	case Boolean:
		n, e := strconv.ParseBool(v)
		if e == nil {
			return n
		}
	}
	return v
}

// Update validates changes and surgically writes only target environment lines.
func (e *Editor) Update(changes map[string]any) error {
	return e.UpdateVersion(changes, "")
}

// UpdateVersion applies changes only when expectedVersion still identifies the compose file
// returned by GET /config. Local updates are serialized and the file is checked again just
// before rename to detect an external editor racing this transaction.
func (e *Editor) UpdateVersion(changes map[string]any, expectedVersion string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	state := e.probeLocked()
	if !state.Available {
		return fmt.Errorf("%w: %s", ErrReadOnly, state.Reason)
	}
	values := map[string]string{}
	for key, v := range changes {
		c, ok := byEnv(key)
		if !ok {
			return fmt.Errorf("unknown setting %s", key)
		}
		if c.ReadOnly {
			return fmt.Errorf("setting %s is read-only", key)
		}
		if c.Masked && fmt.Sprint(v) == "•••" {
			return fmt.Errorf("%s: enter a new value; the write-only placeholder cannot be saved", key)
		}
		s, err := validate(v, c)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		values[key] = s
	}
	b, err := os.ReadFile(e.ComposeFile)
	if err != nil {
		return err
	}
	version := fileVersion(b)
	if expectedVersion != "" && expectedVersion != version {
		return fmt.Errorf("%w: expected %s, found %s", ErrConflict, expectedVersion, version)
	}
	if err := validateVoiceChatDistances(values, string(b), e.Service); err != nil {
		return err
	}
	updated, err := surgery(string(b), e.Service, values)
	if err != nil {
		return err
	}
	if err := validateComposeChange(b, []byte(updated), e.Service, values); err != nil {
		return err
	}
	day := time.Now().Format("2006-01-02")
	bak := e.ComposeFile + "." + day + ".palhelm.bak"
	if _, err = os.Stat(bak); os.IsNotExist(err) {
		if err = os.WriteFile(bak, b, 0o600); err != nil {
			return err
		}
	}
	info, err := os.Stat(e.ComposeFile)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(e.ComposeFile), ".palhelm-compose-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(info.Mode().Perm()); err == nil {
		_, err = tmp.WriteString(updated)
	}
	if er := tmp.Sync(); err == nil {
		err = er
	}
	if er := tmp.Close(); err == nil {
		err = er
	}
	if err != nil {
		return err
	}
	current, err := os.ReadFile(e.ComposeFile)
	if err != nil {
		return err
	}
	if fileVersion(current) != version {
		return fmt.Errorf("%w while writing; reload configuration and retry", ErrConflict)
	}
	if err := os.Rename(name, e.ComposeFile); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(e.ComposeFile))
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr := dir.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func validate(v any, c CatalogEntry) (string, error) {
	s := fmt.Sprint(v)
	for _, r := range s {
		if r == '\r' || r == '\n' || r < 0x20 || r == 0x7f {
			return "", errors.New("must not contain control characters or newlines")
		}
	}
	switch c.Type {
	case Integer:
		if _, e := strconv.ParseInt(s, 10, 64); e != nil {
			return "", errors.New("must be an integer")
		}
	case Number:
		if _, e := strconv.ParseFloat(s, 64); e != nil {
			return "", errors.New("must be a number")
		}
	case Boolean:
		b, e := strconv.ParseBool(s)
		if e != nil {
			return "", errors.New("must be true or false")
		}
		s = strconv.FormatBool(b)
	}
	if c.Env == "PORT" || c.Env == "PUBLIC_PORT" {
		n, _ := strconv.Atoi(s)
		if n < 1 || n > 65535 {
			return "", errors.New("must be between 1 and 65535")
		}
	}
	if c.Env == "PLAYERS" {
		n, _ := strconv.Atoi(s)
		if n < 1 || n > 32 {
			return "", errors.New("must be between 1 and 32")
		}
	}
	return s, nil
}

// validateVoiceChatDistances is the catalog's one cross-field rule: the "zero volume" distance
// must be at or beyond the "max volume" distance, otherwise voice would go silent before it
// even reaches full volume. The single-value validate() above has no visibility into sibling
// fields, so this runs separately in Update, using whichever of the pair is being changed plus
// the other's current compose value (or its catalog default if it isn't set at all yet). It
// only runs when the update actually touches one of the two keys.
func validateVoiceChatDistances(values map[string]string, composeDoc, service string) error {
	const maxKey, zeroKey = "VOICE_CHAT_MAX_VOLUME_DISTANCE", "VOICE_CHAT_ZERO_VOLUME_DISTANCE"
	_, touchesMax := values[maxKey]
	_, touchesZero := values[zeroKey]
	if !touchesMax && !touchesZero {
		return nil
	}
	desired, _, err := parseEnvironment(composeDoc, service)
	if err != nil {
		desired = map[string]string{}
	}
	maxV, err := resolveVoiceDistance(maxKey, values, desired)
	if err != nil {
		return nil // can't resolve the sibling value; leave it to individual field validation
	}
	zeroV, err := resolveVoiceDistance(zeroKey, values, desired)
	if err != nil {
		return nil
	}
	if zeroV < maxV {
		return fmt.Errorf("%s (%g) must be >= %s (%g)", zeroKey, zeroV, maxKey, maxV)
	}
	return nil
}
func resolveVoiceDistance(key string, values, desired map[string]string) (float64, error) {
	if s, ok := values[key]; ok {
		return strconv.ParseFloat(s, 64)
	}
	if s, ok := desired[key]; ok {
		if n, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(s), `"'`), 64); err == nil {
			return n, nil
		}
	}
	if c, ok := byEnv(key); ok {
		if f, ok := c.Default.(float64); ok {
			return f, nil
		}
	}
	return 0, fmt.Errorf("cannot resolve %s", key)
}

var keyRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*:`)

func indent(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }
func parseEnvironment(doc, service string) (map[string]string, [2]int, error) {
	lines := strings.SplitAfter(doc, "\n")
	serviceStart := -1
	serviceIndent := -1
	inServices := false
	servicesIndent := 0
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		ind := indent(line)
		if trim == "services:" {
			inServices = true
			servicesIndent = ind
			continue
		}
		if inServices && ind <= servicesIndent {
			inServices = false
		}
		if inServices && trim == service+":" {
			serviceStart = i
			serviceIndent = ind
			break
		}
	}
	if serviceStart < 0 {
		return nil, [2]int{}, fmt.Errorf("service %q not found", service)
	}
	envStart := -1
	envIndent := 0
	end := len(lines)
	for i := serviceStart + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		ind := indent(lines[i])
		if ind <= serviceIndent {
			end = i
			break
		}
		if trim == "environment:" {
			envStart = i
			envIndent = ind
			break
		}
	}
	if envStart < 0 {
		return nil, [2]int{}, fmt.Errorf("service %q has no environment mapping", service)
	}
	end = len(lines)
	out := map[string]string{}
	for i := envStart + 1; i < len(lines); i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		ind := indent(lines[i])
		if ind <= envIndent {
			end = i
			break
		}
		if strings.HasPrefix(trim, "-") {
			return nil, [2]int{}, errors.New("environment must use YAML mapping form")
		}
		m := keyRE.FindStringSubmatch(trim)
		if len(m) == 0 {
			continue
		}
		colon := strings.Index(trim, ":")
		value := stripComment(strings.TrimSpace(trim[colon+1:]))
		out[m[1]] = decodeYAMLScalar(value)
	}
	return out, [2]int{envStart, end}, nil
}

func decodeYAMLScalar(value string) string {
	var decoded any
	if err := yaml.Unmarshal([]byte(value), &decoded); err == nil {
		switch v := decoded.(type) {
		case nil:
			return ""
		case string:
			return v
		case bool, int, int64, uint64, float64:
			return fmt.Sprint(v)
		}
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}
func stripComment(v string) string {
	raw, _ := splitYAMLComment(v)
	return raw
}

func surgery(doc, service string, changes map[string]string) (string, error) {
	_, span, err := parseEnvironment(doc, service)
	if err != nil {
		return "", err
	}
	lines := strings.SplitAfter(doc, "\n")
	found := map[string]bool{}
	childIndent := ""
	for i := span[0] + 1; i < span[1]; i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		m := keyRE.FindStringSubmatch(trim)
		if len(m) == 0 {
			continue
		}
		if childIndent == "" {
			childIndent = line[:indent(line)]
		}
		value, ok := changes[m[1]]
		if !ok {
			continue
		}
		found[m[1]] = true
		lines[i] = replaceValue(line, m[1], value)
	}
	if childIndent == "" {
		childIndent = strings.Repeat(" ", indent(lines[span[0]])+2)
	}
	missing := []string{}
	for k, v := range changes {
		if !found[k] {
			missing = append(missing, childIndent+k+": "+yamlScalar(k, v)+"\n")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		insertion := strings.Join(missing, "")
		lines = append(lines[:span[1]], append([]string{insertion}, lines[span[1]:]...)...)
	}
	return strings.Join(lines, ""), nil
}
func replaceValue(line, key, value string) string {
	newline := ""
	if strings.HasSuffix(line, "\n") {
		newline = "\n"
		line = strings.TrimSuffix(line, "\n")
	}
	colon := strings.Index(line, ":")
	tail := line[colon+1:]
	_, comment := splitYAMLComment(tail)
	lead := tail[:len(tail)-len(strings.TrimLeft(tail, " "))]
	if comment != "" {
		comment = " " + comment
	}
	return line[:colon+1] + lead + yamlScalar(key, value) + comment + newline
}
func splitYAMLComment(v string) (string, string) {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(v); i++ {
		b := v[i]
		if quote == '"' && b == '\\' && !escaped {
			escaped = true
			continue
		}
		if (b == '"' || b == '\'') && !escaped {
			if quote == 0 {
				quote = b
			} else if quote == b {
				quote = 0
			}
		}
		escaped = false
		if b == '#' && quote == 0 && (i == 0 || v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i]), strings.TrimSpace(v[i:])
		}
	}
	return strings.TrimSpace(v), ""
}

func yamlScalar(key, value string) string {
	entry, ok := byEnv(key)
	if !ok || entry.Type == String {
		return strconv.Quote(value)
	}
	return value
}

func validateComposeChange(before, after []byte, service string, changes map[string]string) error {
	var original, updated map[string]any
	if err := yaml.Unmarshal(before, &original); err != nil {
		return fmt.Errorf("parse existing compose document: %w", err)
	}
	if err := yaml.Unmarshal(after, &updated); err != nil {
		return fmt.Errorf("parse updated compose document: %w", err)
	}
	want := cloneMap(original)
	environment, err := composeEnvironment(want, service)
	if err != nil {
		return err
	}
	for key, value := range changes {
		var scalar any
		if err := yaml.Unmarshal([]byte(yamlScalar(key, value)), &scalar); err != nil {
			return fmt.Errorf("encode %s as YAML scalar: %w", key, err)
		}
		environment[key] = scalar
	}
	if !reflect.DeepEqual(want, updated) {
		return errors.New("updated compose document changed structure outside the requested environment values")
	}
	return nil
}

func composeEnvironment(doc map[string]any, service string) (map[string]any, error) {
	services, ok := doc["services"].(map[string]any)
	if !ok {
		return nil, errors.New("compose document has no services mapping")
	}
	svc, ok := services[service].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("service %q is not a mapping", service)
	}
	environment, ok := svc["environment"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("service %q environment is not a mapping", service)
	}
	return environment, nil
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneMap(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = cloneValue(v[i])
		}
		return out
	default:
		return v
	}
}

func parseINI(text string) map[string]string {
	out := map[string]string{}
	start := strings.Index(text, "OptionSettings=(")
	if start < 0 {
		return out
	}
	s := text[start+len("OptionSettings=("):]
	if end := strings.LastIndex(s, ")"); end >= 0 {
		s = s[:end]
	}
	scan := bufio.NewScanner(strings.NewReader(s))
	scan.Split(splitComma)
	for scan.Scan() {
		part := strings.TrimSpace(scan.Text())
		if eq := strings.Index(part, "="); eq > 0 {
			out[strings.TrimSpace(part[:eq])] = strings.Trim(strings.TrimSpace(part[eq+1:]), `"`)
		}
	}
	return out
}
func splitComma(data []byte, atEOF bool) (int, []byte, error) {
	q := false
	for i, b := range data {
		if b == '"' {
			q = !q
		}
		if b == ',' && !q {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Apply is intentionally disabled. Running Compose inside the Palhelm container cannot
// generally preserve the host project directory, relative bind-path resolution, or project
// identity, so v0.3.0 requires the documented host-side command.
func (e *Editor) Apply(ctx context.Context) (string, error) {
	_ = ctx
	return "", ErrApplyDisabled
}
