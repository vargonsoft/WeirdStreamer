package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Database  DatabaseConfig  `json:"database"`
	Media     MediaConfig     `json:"media"`
	Auth      AuthConfig      `json:"auth"`
	Streaming StreamingConfig `json:"streaming"`
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
	Groups []string `json:"groups"` // Groups that have access to this library
}

type MediaConfig struct {
	Paths                 []MediaPath `json:"paths"`
	ThumbnailPath         string      `json:"thumbnailPath"`
	TrickplayPath         string      `json:"trickplayPath,omitempty"`
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

	GlobalConfig = config
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
