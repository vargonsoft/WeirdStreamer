package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Database  DatabaseConfig  `json:"database"`
	Media     MediaConfig     `json:"media"`
	Auth      AuthConfig      `json:"auth"`
	Streaming StreamingConfig `json:"streaming"`
	Metadata  MetadataConfig  `json:"metadata"`
}

// MetadataConfig configures lookups for "movie" and "tv" libraries.
type MetadataConfig struct {
	TMDBAccessToken string `json:"tmdbAccessToken"` // TMDB API read access token (Bearer)
	Language        string `json:"language"`        // e.g. "en-US"; empty means en-US
}

type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

type DatabaseConfig struct {
	UserPath  string `json:"userPath"`
	MediaPath string `json:"mediaPath"`
}

type MediaPath struct {
	Name   string   `json:"name"`
	Path   string   `json:"path"`
	ID     string   `json:"id,omitempty"`
	Groups []string `json:"groups"`         // Groups that have access to this library
	Type   string   `json:"type,omitempty"` // Optional: LibraryTypeMovie or LibraryTypeTV
}

// Library types. A library has at most one type; untyped libraries are plain folders.
const (
	LibraryTypeMovie = "movie"
	LibraryTypeTV    = "tv"
)

// Validate checks values that can't be expressed by the JSON structure alone.
func (c *Config) Validate() error {
	for _, p := range c.Media.Paths {
		switch p.Type {
		case "", LibraryTypeMovie, LibraryTypeTV:
		default:
			return fmt.Errorf("media path %q: invalid type %q (allowed: %q, %q, or none)", p.Name, p.Type, LibraryTypeMovie, LibraryTypeTV)
		}
	}
	return nil
}

type MediaConfig struct {
	Paths                 []MediaPath `json:"paths"`
	ThumbnailPath         string      `json:"thumbnailPath"`
	TrickplayPath         string      `json:"trickplayPath,omitempty"`
	ArtworkPath           string      `json:"artworkPath,omitempty"`  // Downloaded TMDB posters, backdrops, etc.
	DownloadPath          string      `json:"downloadPath,omitempty"` // Temporary encodes for offline downloads
	SupportedVideoFormats []string    `json:"supportedVideoFormats"`
	SupportedImageFormats []string    `json:"supportedImageFormats"`
	SupportedDocFormats   []string    `json:"supportedDocFormats"`
	IgnoreMacFiles        bool        `json:"ignoreMacFiles"`
}

type AuthConfig struct {
	JWTSecret        string `json:"jwtSecret"`
	TokenExpiryHours int    `json:"tokenExpiryHours"`
}

type StreamingConfig struct {
	ChunkSize         int  `json:"chunkSize"`
	EnableHTTP2       bool `json:"enableHTTP2"`
	AutoConvertImages bool `json:"autoConvertImages"`
}

var GlobalConfig *Config

// loadedPath is the file GlobalConfig was last loaded from, so saves go back to it.
var loadedPath string

// Path returns the file the current configuration was loaded from.
func Path() string {
	return loadedPath
}

func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// First try to unmarshal to detect format
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return err
	}

	config := &Config{}

	// Check if we need to migrate from old format
	needsMigration := false
	if mediaConfig, ok := rawConfig["media"].(map[string]interface{}); ok {
		if paths, ok := mediaConfig["paths"].([]interface{}); ok && len(paths) > 0 {
			if _, isString := paths[0].(string); isString {
				needsMigration = true
			}
		}
	}

	if needsMigration {
		// Load as old format
		var oldConfig struct {
			Server   ServerConfig   `json:"server"`
			Database DatabaseConfig `json:"database"`
			Media    struct {
				Paths                 []string `json:"paths"`
				ThumbnailPath         string   `json:"thumbnailPath"`
				SupportedVideoFormats []string `json:"supportedVideoFormats"`
				SupportedImageFormats []string `json:"supportedImageFormats"`
				SupportedDocFormats   []string `json:"supportedDocFormats"`
			} `json:"media"`
			Auth      AuthConfig      `json:"auth"`
			Streaming StreamingConfig `json:"streaming"`
		}

		if err := json.Unmarshal(data, &oldConfig); err != nil {
			return err
		}

		// Migrate to new format
		config.Server = oldConfig.Server
		config.Database = oldConfig.Database
		config.Auth = oldConfig.Auth
		config.Streaming = oldConfig.Streaming

		config.Media.ThumbnailPath = oldConfig.Media.ThumbnailPath
		config.Media.SupportedVideoFormats = oldConfig.Media.SupportedVideoFormats
		config.Media.SupportedImageFormats = oldConfig.Media.SupportedImageFormats
		config.Media.SupportedDocFormats = oldConfig.Media.SupportedDocFormats

		config.Media.Paths = make([]MediaPath, len(oldConfig.Media.Paths))
		for i, p := range oldConfig.Media.Paths {
			mp := MediaPath{
				Name: filepath.Base(p),
				Path: p,
			}
			// Generate stable ID from the absolute path
			if abs, err := filepath.Abs(p); err == nil {
				sum := sha1.Sum([]byte(abs))
				mp.ID = hex.EncodeToString(sum[:])[:8]
			}
			config.Media.Paths[i] = mp
		}
	} else {
		// Load as new format
		if err := json.Unmarshal(data, config); err != nil {
			return err
		}
		// Ensure each configured media path has a stable ID (for clients)
		for i := range config.Media.Paths {
			if config.Media.Paths[i].ID == "" {
				if abs, err := filepath.Abs(config.Media.Paths[i].Path); err == nil {
					sum := sha1.Sum([]byte(abs))
					config.Media.Paths[i].ID = hex.EncodeToString(sum[:])[:8]
				}
			}
		}
	}

	if err := config.Validate(); err != nil {
		return err
	}

	GlobalConfig = config
	loadedPath = path
	return nil
}

func Save(path string, config *Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(config)
}
