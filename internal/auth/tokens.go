package auth

import (
	"fmt"
	"strings"
)

const TokenMapVersion1 = 1

type Identity struct {
	Name   string
	UID    uint32
	GID    uint32
	Groups []uint32
}

type Entry struct {
	Name   string   `toml:"name"`
	Token  string   `toml:"token"`
	UID    *uint32  `toml:"uid"`
	GID    *uint32  `toml:"gid"`
	Groups []uint32 `toml:"groups"`
}

type TokenMap struct {
	identities map[string]Identity
}

func NewTokenMapFromEntries(source string, entries []Entry) (*TokenMap, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	if strings.TrimSpace(source) == "" {
		source = "token map"
	}

	tokenMap := &TokenMap{
		identities: make(map[string]Identity, len(entries)),
	}

	for index, entry := range entries {
		token := strings.TrimSpace(entry.Token)
		if token == "" {
			return nil, fmt.Errorf("parse %s: tokens[%d].token is required", source, index)
		}
		if entry.UID == nil {
			return nil, fmt.Errorf("parse %s: tokens[%d].uid is required", source, index)
		}
		if entry.GID == nil {
			return nil, fmt.Errorf("parse %s: tokens[%d].gid is required", source, index)
		}
		if _, exists := tokenMap.identities[token]; exists {
			return nil, fmt.Errorf("parse %s: duplicate token at tokens[%d]", source, index)
		}

		tokenMap.identities[token] = Identity{
			Name:   strings.TrimSpace(entry.Name),
			UID:    *entry.UID,
			GID:    *entry.GID,
			Groups: normalizeGroups(entry.Groups, *entry.GID),
		}
	}

	return tokenMap, nil
}

func NewTokenMap(entries map[string]Identity) *TokenMap {
	identities := make(map[string]Identity, len(entries))
	for token, identity := range entries {
		identities[token] = Identity{
			Name:   identity.Name,
			UID:    identity.UID,
			GID:    identity.GID,
			Groups: normalizeGroups(identity.Groups, identity.GID),
		}
	}

	return &TokenMap{identities: identities}
}

func (m *TokenMap) Resolve(token string) (Identity, bool) {
	if m == nil {
		return Identity{}, false
	}

	identity, ok := m.identities[token]
	return identity, ok
}

func normalizeGroups(groups []uint32, primaryGID uint32) []uint32 {
	if len(groups) == 0 {
		return nil
	}

	seen := make(map[uint32]struct{}, len(groups))
	normalized := make([]uint32, 0, len(groups))
	for _, group := range groups {
		if group == primaryGID {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}

	return normalized
}
