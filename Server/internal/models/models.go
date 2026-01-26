package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	IsAdmin      bool      `json:"is_admin"`
}

type MediaItem struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // video, image, document
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
	Duration     float64   `json:"duration,omitempty"` // For videos
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Directory struct {
	Path  string      `json:"path"`
	Name  string      `json:"name"`
	Items []MediaItem `json:"items,omitempty"`
	Dirs  []Directory `json:"dirs,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username" example:"admin"`
	Password string `json:"password" example:"password123"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      User   `json:"user"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
