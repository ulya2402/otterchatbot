package service

import (
	"log"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"sync"
	"time"
)

type AFKService struct {
	UserRepo     *repository.UserRepository
	Bot          *telegram.Client
	I18n         *i18n.I18nService
	lastActivity map[int64]time.Time
	mu           sync.RWMutex
}

func NewAFKService(repo *repository.UserRepository, bot *telegram.Client, i18n *i18n.I18nService) *AFKService {
	return &AFKService{
		UserRepo:     repo,
		Bot:          bot,
		I18n:         i18n,
		lastActivity: make(map[int64]time.Time),
	}
}

// StartWorker menjalankan pengecekan setiap 1 menit
func (s *AFKService) Start() {
	log.Println("AFK Monitor service started...")
	ticker := time.NewTicker(1 * time.Minute)

	for range ticker.C {
		s.checkAFK()
	}
}

// Touch menandakan user sedang aktif (chatting)
func (s *AFKService) Touch(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity[userID] = time.Now()
}

// Stop menghapus user dari pantauan (saat /stop atau left)
func (s *AFKService) Stop(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lastActivity, userID)
}

func (s *AFKService) checkAFK() {
	s.mu.RLock()
	activeUsers := make(map[int64]time.Time)
	for k, v := range s.lastActivity {
		activeUsers[k] = v
	}
	s.mu.RUnlock()

	now := time.Now()

	for userID, lastSeen := range activeUsers {
		duration := now.Sub(lastSeen)
		minutes := int(duration.Minutes())

		// LOGIKA: Cuma 2 kali peringatan
		
		// 1. Peringatan Pertama (Menit ke-5)
		if minutes == 5 {
			s.sendAlert(userID, "afk_alert_1")
		}

		// 2. Peringatan Kedua & Terakhir (Menit ke-10)
		if minutes == 20 {
			s.sendAlert(userID, "afk_alert_2")
		}

		// Jika sudah menit ke-11 ke atas, bot akan diam saja.
	}
}

func (s *AFKService) sendAlert(userID int64, key string) {
	// Cek DB dulu, pastikan user MASIH status chatting
	user, err := s.UserRepo.GetByTelegramID(userID)
	if err != nil || user == nil || user.Status != "chatting" {
		// Jika ternyata sudah tidak chat, hapus dari memori
		s.Stop(userID)
		return
	}

	msg := s.I18n.Get(user.LanguageCode, key)
	_, _ = s.Bot.SendMessage(userID, msg)
}