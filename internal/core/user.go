package core

import "time"

type User struct {
	ID            int64     `json:"id,omitempty"`
	TelegramID    int64     `json:"telegram_id"`
	Username      string    `json:"username"`
	FirstName     string    `json:"first_name"`
	LanguageCode  string    `json:"language_code"`
	Gender        string    `json:"gender"`
	Preference    string    `json:"preference"`
	CurrentMood   string    `json:"current_mood"`
	Status        string    `json:"status"`
	PartnerID     int64     `json:"partner_id,omitempty"`
	IsVIP         bool      `json:"is_vip"`
	Location      string    `json:"location"`       
	LastMessageID int       `json:"last_message_id"` 
	VipExpiresAt  *time.Time `json:"vip_expires_at"`  // Pointer biar bisa NULL
	LastPartnerID int64      `json:"last_partner_id"` // Simpan mantan
	CreatedAt     time.Time `json:"created_at,omitempty"`
}