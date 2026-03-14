package models

import (
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	Group        string    `json:"group"` // The single group the user belongs to
	CreatedAt    time.Time `json:"created_at"`
}

type MediaItem struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // video, image, document
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Duration     float64   `json:"duration"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TrickplayFrame struct {
	Time int    `json:"time"`
	URL  string `json:"url"`
}

type TrickplayInfo struct {
	Frames []TrickplayFrame `json:"frames"`
}

type Directory struct {
	Path  string      `json:"path"`
	Name  string      `json:"name"`
	Items []MediaItem `json:"items"`
	Dirs  []Directory `json:"dirs"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      User   `json:"user"`
}

type UserUpdateRequest struct {
	IsAdmin *bool   `json:"is_admin,omitempty"`
	Group   *string `json:"group,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
