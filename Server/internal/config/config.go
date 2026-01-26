package config

import (
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
	Name string `json:"name"`
	Path string `json:"path"`
}

type MediaConfig struct {
	Paths                 []MediaPath `json:"paths"`
	ThumbnailPath         string      `json:"thumbnailPath"`
	SupportedVideoFormats []string    `json:"supportedVideoFormats"`
	SupportedImageFormats []string    `json:"supportedImageFormats"`
	SupportedDocFormats   []string    `json:"supportedDocFormats"`
}

type AuthConfig struct {
	JWTSecret        string `json:"jwtSecret"`
	TokenExpiryHours int    `json:"tokenExpiryHours"`
}

type StreamingConfig struct {
	ChunkSize   int  `json:"chunkSize"`
	EnableHTTP2 bool `json:"enableHTTP2"`
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
			Server    ServerConfig `json:"server"`
			Database  DatabaseConfig `json:"database"`
			Media     struct {
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
			config.Media.Paths[i] = MediaPath{
				Name: filepath.Base(p),
				Path: p,
			}
		}
	} else {
		// Load as new format
		if err := json.Unmarshal(data, config); err != nil {
			return err
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
