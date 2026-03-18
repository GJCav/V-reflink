package auth

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const TokenMapVersion1 = 1

type Identity struct {
	Name   string
	UID    uint32
	GID    uint32
	Groups []uint32
}

type TokenMap struct {
	identities map[string]Identity
}

type tokenMapFile struct {
	Version int             `yaml:"version"`
	Tokens  []tokenMapEntry `yaml:"tokens"`
}

type tokenMapEntry struct {
	Name   string   `yaml:"name"`
	Token  string   `yaml:"token"`
	UID    *uint32  `yaml:"uid"`
	GID    *uint32  `yaml:"gid"`
	Groups []uint32 `yaml:"groups"`
}

func LoadTokenMap(path string) (*TokenMap, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("read token map %s: %w", path, err)
	}

	var file tokenMapFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse token map %s: %w", path, err)
	}

	if file.Version != TokenMapVersion1 {
		return nil, fmt.Errorf("parse token map %s: unsupported version: %d", path, file.Version)
	}

	tokenMap := &TokenMap{
		identities: make(map[string]Identity, len(file.Tokens)),
	}

	for index, entry := range file.Tokens {
		token := strings.TrimSpace(entry.Token)
		if token == "" {
			return nil, fmt.Errorf("parse token map %s: tokens[%d].token is required", path, index)
		}
		if entry.UID == nil {
			return nil, fmt.Errorf("parse token map %s: tokens[%d].uid is required", path, index)
		}
		if entry.GID == nil {
			return nil, fmt.Errorf("parse token map %s: tokens[%d].gid is required", path, index)
		}
		if _, exists := tokenMap.identities[token]; exists {
			return nil, fmt.Errorf("parse token map %s: duplicate token at tokens[%d]", path, index)
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
