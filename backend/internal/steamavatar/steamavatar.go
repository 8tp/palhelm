// Package steamavatar resolves and caches Steam profile avatars for Palworld
// players, keyed by the REST userId Palhelm already stores (e.g. "steam_7656…").
//
// Avatars are proxied through Palhelm rather than linked directly: the panel's
// Content-Security-Policy is img-src 'self', and proxying also keeps a viewer's
// browser from calling Steam. When STEAM_WEB_API_KEY is set the Web API resolves
// the avatar URL; otherwise the keyless public community endpoint is used. Only
// Steam identities resolve — console (Xbox/PlayStation) crossplay IDs return
// not-found, which the frontend renders as the neutral placeholder.
package steamavatar

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	positiveTTL   = 24 * time.Hour
	negativeTTL   = 1 * time.Hour
	maxImageBytes = 2 << 20 // 2 MiB; Steam avatars are small JP/PNGs.
	fetchTimeout  = 6 * time.Second
)

// steamID64 matches an individual-account SteamID64 (17 digits, 7656119… range).
var steamID64Re = regexp.MustCompile(`^7656119[0-9]{10}$`)

// SteamID64 extracts a bare SteamID64 from a stored REST userId such as
// "steam_76561198012345678". It returns false for console/non-Steam identities.
func SteamID64(rawUserID string) (string, bool) {
	id := strings.TrimSpace(rawUserID)
	id = strings.TrimPrefix(strings.ToLower(id), "steam_")
	if steamID64Re.MatchString(id) {
		return id, true
	}
	return "", false
}

type entry struct {
	data        []byte
	contentType string
	ok          bool
	expiresAt   time.Time
}

// Resolver caches resolved avatar images in memory with positive/negative TTLs.
// A per-SteamID lock collapses concurrent fetches for the same avatar.
type Resolver struct {
	apiKey string
	http   *http.Client
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]entry
	locks map[string]*sync.Mutex
}

func New(apiKey string) *Resolver {
	return &Resolver{
		apiKey: strings.TrimSpace(apiKey),
		http:   &http.Client{Timeout: fetchTimeout},
		now:    time.Now,
		cache:  map[string]entry{},
		locks:  map[string]*sync.Mutex{},
	}
}

// Image returns the cached avatar bytes for a stored REST userId. ok is false for
// non-Steam identities, private/missing profiles, or transient fetch failures
// (negatively cached so a broken profile is not refetched on every request).
func (r *Resolver) Image(ctx context.Context, rawUserID string) (data []byte, contentType string, ok bool) {
	id, valid := SteamID64(rawUserID)
	if !valid {
		return nil, "", false
	}
	if e, hit := r.get(id); hit {
		return e.data, e.contentType, e.ok
	}

	// Serialize concurrent misses for the same avatar behind a per-ID lock.
	lock := r.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	if e, hit := r.get(id); hit {
		return e.data, e.contentType, e.ok
	}

	data, contentType, ok = r.fetch(ctx, id)
	ttl := positiveTTL
	if !ok {
		ttl = negativeTTL
	}
	r.set(id, entry{data: data, contentType: contentType, ok: ok, expiresAt: r.now().Add(ttl)})
	return data, contentType, ok
}

func (r *Resolver) get(id string) (entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[id]
	if !ok || r.now().After(e.expiresAt) {
		return entry{}, false
	}
	return e, true
}

func (r *Resolver) set(id string, e entry) {
	r.mu.Lock()
	r.cache[id] = e
	r.mu.Unlock()
}

func (r *Resolver) lockFor(id string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.locks[id]; ok {
		return l
	}
	l := &sync.Mutex{}
	r.locks[id] = l
	return l
}

func (r *Resolver) fetch(ctx context.Context, id string) ([]byte, string, bool) {
	url, ok := r.resolveURL(ctx, id)
	if !ok || url == "" {
		return nil, "", false
	}
	return r.fetchImage(ctx, url)
}

// resolveURL returns the full-size avatar URL for a SteamID64.
func (r *Resolver) resolveURL(ctx context.Context, id string) (string, bool) {
	if r.apiKey != "" {
		if url, ok := r.resolveViaWebAPI(ctx, id); ok {
			return url, true
		}
		// Fall through to the keyless endpoint if the Web API hiccups.
	}
	return r.resolveViaCommunity(ctx, id)
}

type webAPIResponse struct {
	Response struct {
		Players []struct {
			AvatarFull string `json:"avatarfull"`
		} `json:"players"`
	} `json:"response"`
}

func (r *Resolver) resolveViaWebAPI(ctx context.Context, id string) (string, bool) {
	url := fmt.Sprintf("https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s", r.apiKey, id)
	body, ok := r.getBody(ctx, url, 64<<10)
	if !ok {
		return "", false
	}
	var parsed webAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Response.Players) == 0 {
		return "", false
	}
	return parsed.Response.Players[0].AvatarFull, parsed.Response.Players[0].AvatarFull != ""
}

type communityProfile struct {
	AvatarFull string `xml:"avatarFull"`
}

func (r *Resolver) resolveViaCommunity(ctx context.Context, id string) (string, bool) {
	url := fmt.Sprintf("https://steamcommunity.com/profiles/%s/?xml=1", id)
	body, ok := r.getBody(ctx, url, 256<<10)
	if !ok {
		return "", false
	}
	var parsed communityProfile
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return "", false
	}
	avatar := strings.TrimSpace(parsed.AvatarFull)
	// The XML wraps the URL in CDATA; encoding/xml already unwraps it.
	return avatar, avatar != ""
}

func (r *Resolver) fetchImage(ctx context.Context, url string) ([]byte, string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", false
	}
	return data, contentType, true
}

func (r *Resolver) getBody(ctx context.Context, url string, limit int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Palhelm/1.0 (+player avatar proxy)")
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, false
	}
	return body, true
}
