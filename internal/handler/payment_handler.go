package handler

import (
	"fmt"
	"log"
	"otterchatbot/config"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"time"
)

type PaymentHandler struct {
	Bot      *telegram.Client
	UserRepo *repository.UserRepository
	Config   *config.Config
	I18n     *i18n.I18nService
}

func NewPaymentHandler(bot *telegram.Client, userRepo *repository.UserRepository, cfg *config.Config, i18n *i18n.I18nService) *PaymentHandler {
	return &PaymentHandler{
		Bot:      bot,
		UserRepo: userRepo,
		Config:   cfg,
		I18n:     i18n,
	}
}

func (h *PaymentHandler) SendVIPInvoice(chatID int64, planID string, lang string) {
	// 1. Cari Paket di Config
	var selectedPlan *config.VIPPlan
	for _, plan := range h.Config.VIPPlans {
		if plan.ID == planID {
			selectedPlan = &plan
			break
		}
	}

	if selectedPlan == nil {
		log.Printf("Error: Plan ID '%s' not found in pricing.json", planID)
		_, _ = h.Bot.SendMessage(chatID, "❌ Error: Paket tidak ditemukan di sistem.")
		return
	}

	// 2. Ambil Teks
	title := h.I18n.Get(lang, selectedPlan.TitleKey)
	desc := h.I18n.Get(lang, selectedPlan.DescKey)

	// Fallback jika lupa isi locales
	if title == selectedPlan.TitleKey { title = "VIP Access" }
	if desc == selectedPlan.DescKey { desc = "Premium features access" }

	// 3. Buat Request Invoice
	req := telegram.SendInvoiceRequest{
		ChatID:        chatID,
		Title:         title,
		Description:   desc,
		Payload:       selectedPlan.ID,
		Currency:      "XTR", // WAJIB "XTR" untuk Stars
		ProviderToken: "",    // WAJIB KOSONG untuk Stars
		Prices: []telegram.LabeledPrice{
			{Label: title, Amount: selectedPlan.Price},
		},
	}

	// 4. Kirim & Cek Error
	err := h.Bot.SendInvoice(req)
	if err != nil {
		log.Printf("Failed to send invoice: %v", err)
		// Debugging: Kirim pesan error ke user agar tahu salahnya dimana
		errorMsg := fmt.Sprintf("❌ Telegram Refused: %v\n\nCheck BotFather > Payments.", err)
		_, _ = h.Bot.SendMessage(chatID, errorMsg)
	}
}

// HandlePreCheckout (Validasi sebelum bayar)
func (h *PaymentHandler) HandlePreCheckout(query *telegram.PreCheckoutQuery) {
	isValidPlan := false
	for _, plan := range h.Config.VIPPlans {
		if plan.ID == query.InvoicePayload {
			isValidPlan = true
			break
		}
	}

	if !isValidPlan {
		_ = h.Bot.AnswerPreCheckoutQuery(query.ID, false, "Plan no longer exists.")
		return
	}

	// Terima Transaksi
	_ = h.Bot.AnswerPreCheckoutQuery(query.ID, true, "")
}

// HandleSuccessfulPayment (Aktivasi VIP)
func (h *PaymentHandler) HandleSuccessfulPayment(msg *telegram.Message) {
	payment := msg.SuccessfulPayment
	telegramID := msg.From.ID
	chargeID := payment.TelegramPaymentChargeID
	
	user, err := h.UserRepo.GetByTelegramID(telegramID)
	if err != nil || user == nil { return }

	// Security: Anti-Replay
	if user.LastChargeID == chargeID {
		log.Printf("Duplicate payment ignored: %s", chargeID)
		return 
	}

	// Cari durasi hari
	days := 0
	for _, plan := range h.Config.VIPPlans {
		if plan.ID == payment.InvoicePayload {
			days = plan.Days
			break
		}
	}

	if days == 0 {
		log.Printf("Unknown plan payload: %s", payment.InvoicePayload)
		_, _ = h.Bot.SendMessage(telegramID, "⚠️ Error activating VIP. Contact admin.")
		return
	}

	// Hitung Expired Date
	now := time.Now()
	var startTime time.Time
	if user.IsVIP && user.VipExpiresAt != nil && user.VipExpiresAt.After(now) {
		startTime = *user.VipExpiresAt
	} else {
		startTime = now
	}

	expiry := startTime.Add(time.Duration(days) * 24 * time.Hour)
	
	user.IsVIP = true
	user.VipExpiresAt = &expiry
	user.LastChargeID = chargeID

	if err := h.UserRepo.Update(user); err != nil {
		log.Printf("DB Update Failed: %v", err)
		_, _ = h.Bot.SendMessage(telegramID, "⚠️ Database error. Contact admin.")
		return
	}

	successMsg := fmt.Sprintf("🌟 <b>PAYMENT SUCCESSFUL!</b>\n\nVIP Active for <b>%d days</b>.\nEnjoy your features!", days)
	_, _ = h.Bot.SendMessage(telegramID, successMsg)
	
	log.Printf("SUCCESS: User %d bought %d days via Stars.", telegramID, days)
}