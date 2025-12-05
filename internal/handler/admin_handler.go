package handler

import (
	"fmt"
	"otterchatbot/config"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/telegram"
	"strconv"
	"strings"
	"time"
)

type AdminHandler struct {
	Bot      *telegram.Client
	UserRepo *repository.UserRepository
	Config   *config.Config
}

func NewAdminHandler(bot *telegram.Client, userRepo *repository.UserRepository, cfg *config.Config) *AdminHandler {
	return &AdminHandler{
		Bot:      bot,
		UserRepo: userRepo,
		Config:   cfg,
	}
}

// IsAdmin mengecek apakah ID pengirim ada di daftar admin .env
func (h *AdminHandler) IsAdmin(userID int64) bool {
	idStr := fmt.Sprintf("%d", userID)
	for _, adminID := range h.Config.AdminIDs {
		if strings.TrimSpace(adminID) == idStr {
			return true
		}
	}
	return false
}

// HandleCommand memproses perintah admin
func (h *AdminHandler) HandleCommand(msg *telegram.Message) {
	args := strings.Split(msg.Text, " ")
	command := args[0]

	switch command {
	case "/stats":
		h.handleStats(msg.Chat.ID)
	case "/broadcast":
		h.handleBroadcast(msg.Chat.ID, args)
	case "/addvip":
		h.handleAddVIP(msg.Chat.ID, args)
	}
}

func (h *AdminHandler) handleStats(chatID int64) {
	totalUsers, _ := h.UserRepo.CountAll()
	chatting, queue, vips := h.UserRepo.GetLiveStats()

	text := fmt.Sprintf(
		"📊 **REAL-TIME STATS**\n\n"+
			"👥 Total Users: %d\n"+
			"💬 Chatting Pairs: %d\n"+
			"⏳ In Queue: %d\n"+
			"🌟 Total VIP: %d",
		totalUsers, chatting/2, queue, vips,
	)
	_, _ = h.Bot.SendMessage(chatID, text)
}

func (h *AdminHandler) handleBroadcast(chatID int64, args []string) {
	if len(args) < 2 {
		_, _ = h.Bot.SendMessage(chatID, "⚠️ Usage: `/broadcast [message]`")
		return
	}

	// Gabungkan argumen menjadi satu pesan
	message := strings.Join(args[1:], " ")
	
	// Jalankan di Goroutine (Background) agar bot tidak macet
	go func() {
		ids, err := h.UserRepo.GetAllTelegramIDs()
		if err != nil {
			_, _ = h.Bot.SendMessage(chatID, "❌ Error fetching users.")
			return
		}

		_, _ = h.Bot.SendMessage(chatID, fmt.Sprintf("🚀 Broadcasting to %d users...", len(ids)))
		
		success := 0
		fail := 0

		for _, id := range ids {
			// Skip kirim ke admin sendiri (opsional)
			if id == chatID { continue }

			// FIX: Menangkap 2 return value (msgID, error)
			_, err := h.Bot.SendMessage(id, "📢 <b>ANNOUNCEMENT</b>\n\n"+message)
			if err == nil {
				success++
			} else {
				fail++
			}
			
			// Sleep 35ms agar tidak kena limit Telegram (30 pesan/detik)
			time.Sleep(35 * time.Millisecond)
		}

		report := fmt.Sprintf("✅ **Broadcast Done!**\nSuccess: %d\nFailed: %d", success, fail)
		_, _ = h.Bot.SendMessage(chatID, report)
	}()
}

func (h *AdminHandler) handleAddVIP(chatID int64, args []string) {
	// Format: /addvip 12345678 30
	if len(args) < 3 {
		_, _ = h.Bot.SendMessage(chatID, "⚠️ Usage: `/addvip [user_id] [days]`")
		return
	}

	targetIDStr := args[1]
	daysStr := args[2]

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		_, _ = h.Bot.SendMessage(chatID, "❌ Invalid User ID.")
		return
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil {
		_, _ = h.Bot.SendMessage(chatID, "❌ Invalid duration.")
		return
	}

	// Ambil user
	user, err := h.UserRepo.GetByTelegramID(targetID)
	if err != nil || user == nil {
		_, _ = h.Bot.SendMessage(chatID, "❌ User not found in database.")
		return
	}

	// Update VIP
	now := time.Now()
	expiry := now.Add(time.Duration(days) * 24 * time.Hour)
	
	user.IsVIP = true
	user.VipExpiresAt = &expiry
	
	err = h.UserRepo.Update(user)
	if err != nil {
		_, _ = h.Bot.SendMessage(chatID, "❌ Database update failed.")
		return
	}

	// Konfirmasi ke Admin
	_, _ = h.Bot.SendMessage(chatID, fmt.Sprintf("✅ VIP added to %s for %d days.", user.FirstName, days))

	// Notifikasi ke User
	msgUser := fmt.Sprintf("🌟 <b>CONGRATULATIONS!</b>\n\nYour account is now <b>VIP</b> for %d days!\nEnjoy exclusive features.", days)
	_, _ = h.Bot.SendMessage(targetID, msgUser)
}