package handler

import (
	"fmt"
	"log"
	"otterchatbot/config"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"strings"
)

// Gunakan URL yang pasti berakhiran .png/.jpg dan dapat diakses publik
const WelcomeImageURL = "https://upload.wikimedia.org/wikipedia/commons/thumb/8/82/Telegram_logo.svg/1024px-Telegram_logo.svg.png"

type BotHandler struct {
	Bot      *telegram.Client
	UserRepo *repository.UserRepository
	I18n     *i18n.I18nService
	Admin    *AdminHandler
	Payment  *PaymentHandler
}

func NewBotHandler(bot *telegram.Client, userRepo *repository.UserRepository, i18n *i18n.I18nService, cfg *config.Config) *BotHandler {
	return &BotHandler{
		Bot:      bot,
		UserRepo: userRepo,
		I18n:     i18n,
		Admin:    NewAdminHandler(bot, userRepo, cfg),
		// FIX: Update parameter PaymentHandler agar sesuai dengan perubahan sebelumnya
		Payment:  NewPaymentHandler(bot, userRepo, cfg, i18n),
	}
}

func (h *BotHandler) HandleUpdate(update telegram.Update) {
	// 1. Tangani Pembayaran (Prioritas)
	if update.PreCheckoutQuery != nil {
		h.Payment.HandlePreCheckout(update.PreCheckoutQuery)
		return
	}

	if update.Message != nil && update.Message.SuccessfulPayment != nil {
		h.Payment.HandleSuccessfulPayment(update.Message)
		return
	}

	// 2. Tangani Callback
	if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery)
		return
	}

	// 3. Tangani Pesan Teks
	if update.Message != nil {
		h.handleMessage(update.Message)
	}
}

func (h *BotHandler) handleMessage(msg *telegram.Message) {
	telegramID := msg.From.ID
	chatID := msg.Chat.ID

	// 1. Cek Admin (Hanya jika pesan berupa teks command)
	if msg.Text != "" && strings.HasPrefix(msg.Text, "/") && h.Admin.IsAdmin(telegramID) {
		cmd := strings.Split(msg.Text, " ")[0]
		if cmd == "/stats" || cmd == "/broadcast" || cmd == "/addvip" {
			h.Admin.HandleCommand(msg)
			return 
		}
	}
	
	user, err := h.UserRepo.GetByTelegramID(telegramID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	if user == nil {
		h.startOnboarding(msg)
		return
	}

	// FIX: Bersihkan pesan lama hanya jika user mengetik command teks
	if msg.Text != "" && strings.HasPrefix(msg.Text, "/") && user.LastMessageID != 0 {
		_ = h.Bot.DeleteMessage(chatID, user.LastMessageID)
	}

	// Command Stop & Reconnect (Hanya Text)
	if msg.Text == "/stop" {
		h.stopChat(user)
		return
	}
	if msg.Text == "/reconnect" {
		h.handleReconnect(user)
		return
	}

	// Input Lokasi (Hanya Text)
	if user.Status == "awaiting_location" {
		// Jika user kirim stiker pas diminta lokasi, abaikan atau minta teks lagi
		if msg.Text == "" {
			_, _ = h.Bot.SendMessage(chatID, "⚠️ Please send text for your location.")
			return
		}
		user.Location = msg.Text
		user.Status = "idle"
		_ = h.UserRepo.Update(user)
		
		confirmMsg := fmt.Sprintf(h.I18n.Get(user.LanguageCode, "location_saved"), user.Location)
		_, _ = h.Bot.SendMessage(chatID, confirmMsg)
		h.sendUserProfile(chatID, user, false)
		return
	}

	// 2. LOGIKA RELAY (UNTUK SEMUA TIPE PESAN)
	// Stiker/Foto akan masuk ke sini
	if user.Status == "chatting" {
		h.relayMessage(user, msg)
		return
	}

	// 3. Menu Command (Hanya Text)
	// Jika user kirim stiker di menu utama, bot akan diam atau refresh menu (default)
	switch msg.Text {
	case "/start":
		h.sendMainMenu(chatID, user, false, 0)
		
	case "/profile":
		h.sendUserProfile(chatID, user, false)

	case "/vip":
		h.sendVipInfo(chatID, user.LanguageCode, false, 0)

	case "/search":
		h.cleanStatus(user)
		h.sendMoodSelector(chatID, user.LanguageCode, false, 0)

	case "/lang":
		h.sendLangSelector(chatID, user.LanguageCode, false, 0, "profile")

	case "/help":
		h.sendHelpMenu(chatID, user.LanguageCode, false, 0)

	default:
		if user.Status == "queue" {
			_, _ = h.Bot.SendMessage(chatID, "Still searching... Type /stop to cancel.")
		} else {
			// Jika kirim pesan random/stiker di menu utama, refresh menu
			h.sendMainMenu(chatID, user, false, 0)
		}
	}
}

func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func (h *BotHandler) handleReconnect(user *core.User) {
	// Cek VIP
	if !user.IsVIP {
		h.Bot.SendMessage(user.TelegramID, "🔒 <b>VIP Feature</b>\nReconnect is only available for VIP members.")
		return
	}

	if user.LastPartnerID == 0 {
		h.Bot.SendMessage(user.TelegramID, "⚠️ You don't have a previous partner to reconnect with.")
		return
	}

	// Cek status mantan
	partner, err := h.UserRepo.GetByTelegramID(user.LastPartnerID)
	if err != nil || partner == nil {
		h.Bot.SendMessage(user.TelegramID, "⚠️ Previous partner not found.")
		return
	}

	if partner.Status != "idle" {
		h.Bot.SendMessage(user.TelegramID, "⚠️ Previous partner is currently busy (chatting/queueing). Try again later.")
		return
	}

	// EKSEKUSI RECONNECT (Force Match)
	user.Status = "chatting"
	user.PartnerID = partner.TelegramID
	
	partner.Status = "chatting"
	partner.PartnerID = user.TelegramID

	_ = h.UserRepo.Update(user)
	_ = h.UserRepo.Update(partner)

	// Hapus pesan menu lama di kedua belah pihak agar bersih
	if user.LastMessageID != 0 { _ = h.Bot.DeleteMessage(user.TelegramID, user.LastMessageID) }
	if partner.LastMessageID != 0 { _ = h.Bot.DeleteMessage(partner.TelegramID, partner.LastMessageID) }

	h.Bot.SendMessage(user.TelegramID, "🔄 <b>Reconnected!</b> You are back with your previous partner.")
	h.Bot.SendMessage(partner.TelegramID, "🔄 <b>Reconnected!</b> Your previous partner reconnected with you (VIP Feature).")
}

func (h *BotHandler) sendVipInfo(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "vip_info")
	
	var rows [][]telegram.InlineKeyboardButton
	
	// LOOPING DATA DARI CONFIG JSON
	// Ini membuat tombol otomatis muncul sesuai jumlah paket di pricing.json
	// Tanpa perlu ubah kode Go jika nambah paket baru
	for _, plan := range h.Payment.Config.VIPPlans {
		// Format label: "⭐️ 7 Hari (50 Stars)"
		// Mengambil format dari locales (btn_buy_format)
		labelFormat := h.I18n.Get(lang, "btn_buy_format")
		label := fmt.Sprintf(labelFormat, plan.Days, plan.Price)
		
		btn := telegram.InlineKeyboardButton{
			Text:         label,
			CallbackData: "buy:" + plan.ID, // Contoh: buy:vip_weekly
		}
		rows = append(rows, []telegram.InlineKeyboardButton{btn})
	}

	// Tombol manual & back
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: h.I18n.Get(lang, "btn_contact_admin"), Url: h.I18n.Get(lang, "vip_contact_url")},
	})
	rows = append(rows, []telegram.InlineKeyboardButton{
		{Text: "🏠 Main Menu", CallbackData: "back:menu"},
	})

	keyboard := telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
	h.sendOrEdit(chatID, text, keyboard, isEdit, msgID)
}

// --- FUNGSI BARU: MENAMPILKAN MENU HELP INTERAKTIF ---
func (h *BotHandler) sendHelpMenu(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "help_menu")
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: h.I18n.Get(lang, "help_btn_basic"), CallbackData: "help:basic"},
				{Text: h.I18n.Get(lang, "help_btn_cmd"), CallbackData: "help:cmd"},
			},
			{
				{Text: h.I18n.Get(lang, "help_btn_rules"), CallbackData: "help:rules"},
			},
			{
				{Text: "🏠 Main Menu", CallbackData: "back:menu"},
			},
		},
	}
	h.sendOrEdit(chatID, text, keyboard, isEdit, msgID)
}

func (h *BotHandler) cleanStatus(user *core.User) {
	if user.Status == "queue" || user.Status == "idle" {
		user.Status = "idle"
		user.PartnerID = 0
		_ = h.UserRepo.Update(user)
	}
}

// FIX: Tambahkan parameter isEdit dan msgID
func (h *BotHandler) sendMainMenu(chatID int64, user *core.User, isEdit bool, msgID int) {
	caption := h.I18n.Get(user.LanguageCode, "welcome_caption")
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_search"), CallbackData: "cmd:search"},
			},
            // Update baris ini untuk menampilkan tombol VIP
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_profile"), CallbackData: "cmd:profile"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_vip"), CallbackData: "cmd:vip"},
			},
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_help"), CallbackData: "cmd:help"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_lang"), CallbackData: "edit:lang_from_menu"},
			},
		},
	}

	h.sendOrEdit(chatID, caption, keyboard, isEdit, msgID)
}

func (h *BotHandler) sendInfoMessage(chatID int64, lang string, key string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, key)
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🏠 Main Menu", CallbackData: "back:menu"}},
		},
	}
	h.sendOrEdit(chatID, text, keyboard, isEdit, msgID)
}

func (h *BotHandler) sendUserProfile(chatID int64, user *core.User, isEdit bool) {
	viewTemplate := h.I18n.Get(user.LanguageCode, "profile_view")
	
	gender := user.Gender
	if gender == "male" { gender = h.I18n.Get(user.LanguageCode, "btn_male") }
	if gender == "female" { gender = h.I18n.Get(user.LanguageCode, "btn_female") }

	pref := user.Preference
	if pref == "male" { pref = h.I18n.Get(user.LanguageCode, "btn_male") }
	if pref == "female" { pref = h.I18n.Get(user.LanguageCode, "btn_female") }
	if pref == "both" { pref = h.I18n.Get(user.LanguageCode, "btn_both") }

	loc := user.Location
	if loc == "" { loc = "-" }

	statusText := "Free"
	if user.IsVIP { statusText = "🌟 VIP" }

	// FIX: Menggunakan escapeHTML untuk nama user
	text := fmt.Sprintf(viewTemplate, escapeHTML(user.FirstName), gender, pref, loc, statusText)

	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_gender"), CallbackData: "edit:gender"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_pref"), CallbackData: "edit:pref"},
			},
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_loc"), CallbackData: "edit:loc"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_lang"), CallbackData: "edit:lang_from_profile"},
			},
			{
				{Text: "🏠 Main Menu", CallbackData: "back:menu"},
			},
		},
	}

	h.sendOrEdit(chatID, text, keyboard, isEdit, user.LastMessageID)
}

func (h *BotHandler) relayMessage(sender *core.User, msg *telegram.Message) {
	if sender.PartnerID == 0 {
		_, _ = h.Bot.SendMessage(sender.TelegramID, h.I18n.Get(sender.LanguageCode, "partner_lost"))
		sender.Status = "idle"
		_ = h.UserRepo.Update(sender)
		return
	}
	
	// FIX: Gunakan CopyMessage agar mendukung Foto, Stiker, Voice, Video, dll
	// Parameter: (Tujuan, Asal, ID Pesan Asal)
	_, err := h.Bot.CopyMessage(sender.PartnerID, sender.TelegramID, msg.MessageID)
	
	if err != nil {
		log.Printf("Failed to relay message from %d to %d: %v", sender.TelegramID, sender.PartnerID, err)
		
		// Opsional: Cek error spesifik (misal diblokir) sebelum stop chat
		// Tapi untuk keamanan, jika gagal kirim, kita asumsikan putus.
		h.stopChat(sender)
	}
}

func (h *BotHandler) stopChat(initiator *core.User) {
	// 1. IDLE: Jika tidak sedang ngapa-ngapain, langsung kasih menu search
	if initiator.Status == "idle" {
		h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)
		return
	}

	// 2. QUEUE: Jika sedang antri, batalkan antrian
	if initiator.Status == "queue" {
		initiator.Status = "idle"
		initiator.PartnerID = 0 
		_ = h.UserRepo.Update(initiator)

		// Ubah pesan "Searching..." jadi "Cancelled"
		if initiator.LastMessageID != 0 {
			_ = h.Bot.EditMessageText(initiator.TelegramID, initiator.LastMessageID, "⛔ Search cancelled.", nil)
		} else {
			_, _ = h.Bot.SendMessage(initiator.TelegramID, "⛔ Search cancelled.")
		}
		
		h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)
		return
	}

	// 3. CHATTING: Jika sedang chat, putuskan hubungan
	partnerID := initiator.PartnerID
	
	// --- AWAL PERUBAHAN: LOGIKA SIMPAN MANTAN & TOMBOL RECONNECT ---
	
	// Simpan Mantan & Reset Initiator
	initiator.LastPartnerID = partnerID 
	initiator.Status = "idle"
	initiator.PartnerID = 0
	_ = h.UserRepo.Update(initiator)
	
	// Kirim pesan Stop + Tombol Reconnect (Teaser)
	stopText := h.I18n.Get(initiator.LanguageCode, "chat_ended")
	reconnectBtn := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: h.I18n.Get(initiator.LanguageCode, "btn_reconnect"), CallbackData: "cmd:reconnect_teaser"}},
		},
	}
	// Gunakan SendMessageComplex karena ada tombolnya
	_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: initiator.TelegramID, Text: stopText, ReplyMarkup: reconnectBtn, ParseMode: "HTML",
	})

	// Tampilkan Menu Search lagi
	h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)

	// Reset Partner (Korban)
	if partnerID != 0 {
		partner, err := h.UserRepo.GetByTelegramID(partnerID)
		if err == nil && partner != nil && partner.PartnerID == initiator.TelegramID {
			partner.LastPartnerID = initiator.TelegramID
			partner.Status = "idle"
			partner.PartnerID = 0
			_ = h.UserRepo.Update(partner)

			// Kirim pesan Partner Left + Tombol Reconnect (Teaser) ke Partner juga
			stopTextPartner := h.I18n.Get(partner.LanguageCode, "partner_left")
			reconnectBtnPartner := telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: h.I18n.Get(partner.LanguageCode, "btn_reconnect"), CallbackData: "cmd:reconnect_teaser"}},
				},
			}
			_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
				ChatID: partner.TelegramID, Text: stopTextPartner, ReplyMarkup: reconnectBtnPartner, ParseMode: "HTML",
			})

			h.sendMoodSelector(partner.TelegramID, partner.LanguageCode, false, 0)
		}
	}
	// --- AKHIR PERUBAHAN ---
}

func (h *BotHandler) startOnboarding(msg *telegram.Message) {
	newUser := &core.User{
		TelegramID: msg.From.ID, Username: msg.From.Username, FirstName: msg.From.FirstName,
		LanguageCode: msg.From.LanguageCode, Status: "onboarding",
	}
	if newUser.LanguageCode == "" { newUser.LanguageCode = "en" }
	_ = h.UserRepo.Create(newUser)
	
	// Kirim pesan baru (false, 0)
	h.sendMainMenu(msg.Chat.ID, newUser, false, 0)
}

func (h *BotHandler) sendGenderSelector(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "ask_gender")
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: h.I18n.Get(lang, "btn_male"), CallbackData: "gender:male"}, {Text: h.I18n.Get(lang, "btn_female"), CallbackData: "gender:female"}},
	}
	if isEdit {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: h.I18n.Get(lang, "btn_back"), CallbackData: "back:profile"}})
	}

	h.sendOrEdit(chatID, text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, isEdit, msgID)
}

func (h *BotHandler) sendPreferenceSelector(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "ask_preference")
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: h.I18n.Get(lang, "btn_male"), CallbackData: "pref:male"}, {Text: h.I18n.Get(lang, "btn_female"), CallbackData: "pref:female"}},
		{{Text: h.I18n.Get(lang, "btn_both"), CallbackData: "pref:both"}},
	}
	if isEdit {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: h.I18n.Get(lang, "btn_back"), CallbackData: "back:profile"}})
	}

	h.sendOrEdit(chatID, text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, isEdit, msgID)
}

func (h *BotHandler) sendLangSelector(chatID int64, lang string, isEdit bool, msgID int, origin string) {
	text := h.I18n.Get(lang, "ask_lang")
	
	var rows [][]telegram.InlineKeyboardButton
	var currentRow []telegram.InlineKeyboardButton

	for _, l := range core.AvailableLanguages {
		// FIX: Menyisipkan origin ke dalam callback data (setlang:CODE:ORIGIN)
		btnText := fmt.Sprintf("%s %s", l.Icon, l.Label)
		currentRow = append(currentRow, telegram.InlineKeyboardButton{Text: btnText, CallbackData: "setlang:" + l.Code + ":" + origin})
		
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = []telegram.InlineKeyboardButton{}
		}
	}
	if len(currentRow) > 0 { rows = append(rows, currentRow) }

	// FIX: Tentukan tombol Back lari kemana
	backCallback := "back:profile"
	if origin == "menu" {
		backCallback = "back:menu"
	}
	
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: h.I18n.Get(lang, "btn_back"), CallbackData: backCallback}})
	
	h.sendOrEdit(chatID, text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, isEdit, msgID)
}

func (h *BotHandler) sendMoodSelector(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "select_mood")
	
	var rows [][]telegram.InlineKeyboardButton
	var currentRow []telegram.InlineKeyboardButton

	for _, m := range core.AvailableMoods {
		btnText := h.I18n.Get(lang, m.Label)
		currentRow = append(currentRow, telegram.InlineKeyboardButton{Text: btnText, CallbackData: "mood:" + m.Code})
		
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = []telegram.InlineKeyboardButton{}
		}
	}
	if len(currentRow) > 0 { rows = append(rows, currentRow) }
	
	// Tambahkan Tombol Back ke Menu Utama
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "🏠 Main Menu", CallbackData: "back:menu"}})
	
	h.sendOrEdit(chatID, text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, isEdit, msgID)
}

func (h *BotHandler) sendLocationSelector(chatID int64, lang string, isEdit bool, msgID int) {
	text := h.I18n.Get(lang, "ask_location")

	var rows [][]telegram.InlineKeyboardButton
	var currentRow []telegram.InlineKeyboardButton

	for _, c := range core.AvailableCountries {
		btnText := fmt.Sprintf("%s %s", c.Icon, c.Label)
		currentRow = append(currentRow, telegram.InlineKeyboardButton{Text: btnText, CallbackData: "setloc:" + c.Label + "|" + c.Icon})
		
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = []telegram.InlineKeyboardButton{}
		}
	}
	if len(currentRow) > 0 { rows = append(rows, currentRow) }

	rows = append(rows, []telegram.InlineKeyboardButton{{Text: h.I18n.Get(lang, "btn_back"), CallbackData: "back:profile"}})

	h.sendOrEdit(chatID, text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}, isEdit, msgID)
}

func (h *BotHandler) sendOrEdit(chatID int64, text string, markup telegram.InlineKeyboardMarkup, isEdit bool, msgID int) {
	if isEdit {
		_ = h.Bot.EditMessageText(chatID, msgID, text, markup)
	} else {
		newMsgID, _ := h.Bot.SendMessageComplex(telegram.SendMessageRequest{
			ChatID: chatID, Text: text, ReplyMarkup: markup, ParseMode: "HTML",
		})
		if newMsgID != 0 {
			user, _ := h.UserRepo.GetByTelegramID(chatID)
			if user != nil {
				user.LastMessageID = newMsgID
				_ = h.UserRepo.Update(user)
			}
		}
	}
}

func (h *BotHandler) handleCallback(cb *telegram.CallbackQuery) {
	h.Bot.AnswerCallbackQuery(cb.ID, "")
	telegramID := cb.From.ID
	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID
	data := cb.Data

	user, err := h.UserRepo.GetByTelegramID(telegramID)
	if err != nil || user == nil { return }


	if data == "cmd:stop" {
		h.stopChat(user)
		return
	}

	if data == "cmd:reconnect_teaser" {
		if user.IsVIP {
			h.handleReconnect(user)
		} else {
			pitchText := h.I18n.Get(user.LanguageCode, "vip_pitch")
			keyboard := telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: h.I18n.Get(user.LanguageCode, "btn_vip"), CallbackData: "cmd:vip"}},
				},
			}
			_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
				ChatID: chatID, Text: pitchText, ReplyMarkup: keyboard, ParseMode: "HTML",
			})
		}
		return
	}

	// --- [AWAL PERUBAHAN] LOGIKA PEMBAYARAN DINAMIS ---
	// Menangkap tombol "buy:vip_weekly" atau "buy:vip_monthly"
	// WAJIB diletakkan sebelum logika navigasi lain
	if strings.HasPrefix(data, "buy:") {
		// Ambil ID Paket (misal: "vip_weekly") dari callback data
		planID := strings.TrimPrefix(data, "buy:")
		
		// Panggil Payment Handler dengan 3 Parameter: ChatID, PlanID, Bahasa
		h.Payment.SendVIPInvoice(chatID, planID, user.LanguageCode)
		return
	}
	// --- [AKHIR PERUBAHAN] ---

	if data == "cmd:search" {
		_ = h.Bot.DeleteMessage(chatID, msgID) 
		h.cleanStatus(user)
		h.sendMoodSelector(chatID, user.LanguageCode, false, 0)
		return
	}
	if data == "cmd:profile" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendUserProfile(chatID, user, false)
		return
	}
	
	if data == "cmd:vip" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendVipInfo(chatID, user.LanguageCode, false, 0)
		return
	}

	if data == "cmd:help" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendHelpMenu(chatID, user.LanguageCode, false, 0)
		return
	}
	
	if strings.HasPrefix(data, "help:") {
		topic := strings.Split(data, ":")[1]
		contentKey := "help_content_" + topic
		
		text := h.I18n.Get(user.LanguageCode, contentKey)
		keyboard := telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{
				{{Text: "🔙 Back to Help", CallbackData: "back:help_menu"}},
			},
		}
		h.sendOrEdit(chatID, text, keyboard, true, msgID)
		return
	}

	if data == "back:help_menu" {
		h.sendHelpMenu(chatID, user.LanguageCode, true, msgID)
		return
	}

	if data == "cmd:about" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendInfoMessage(chatID, user.LanguageCode, "about_text", false, 0)
		return
	}

	if data == "edit:lang_from_menu" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendLangSelector(chatID, user.LanguageCode, false, 0, "menu")
		return
	}

	if data == "back:menu" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.sendMainMenu(chatID, user, false, 0)
		return
	}

	if data == "back:profile" {
		h.sendUserProfile(chatID, user, true)
		return
	}
	if data == "edit:gender" {
		h.sendGenderSelector(chatID, user.LanguageCode, true, msgID)
		return
	}
	if data == "edit:pref" {
		h.sendPreferenceSelector(chatID, user.LanguageCode, true, msgID)
		return
	}
	if data == "edit:loc" {
		h.sendLocationSelector(chatID, user.LanguageCode, true, msgID)
		return
	}
	if data == "edit:lang_from_profile" {
		h.sendLangSelector(chatID, user.LanguageCode, true, msgID, "profile")
		return
	}

	if strings.HasPrefix(data, "setlang:") {
		parts := strings.Split(data, ":")
		lang := parts[1]
		origin := "menu" 
		if len(parts) > 2 { origin = parts[2] }

		user.LanguageCode = lang
		_ = h.UserRepo.Update(user)

		if origin == "menu" {
			_ = h.Bot.DeleteMessage(chatID, msgID)
			h.sendMainMenu(chatID, user, false, 0)
		} else {
			h.sendUserProfile(chatID, user, true)
		}
	
	} else if strings.HasPrefix(data, "setloc:") {
		rawData := strings.TrimPrefix(data, "setloc:")
		parts := strings.Split(rawData, "|")
		locName := parts[0]
		locIcon := ""
		if len(parts) > 1 { locIcon = parts[1] }
		
		user.Location = fmt.Sprintf("%s %s", locIcon, locName)
		_ = h.UserRepo.Update(user)
		h.sendUserProfile(chatID, user, true)

	} else if strings.HasPrefix(data, "gender:") {
		gender := strings.Split(data, ":")[1]
		user.Gender = gender
		_ = h.UserRepo.Update(user)
		h.sendUserProfile(chatID, user, true) 

	} else if strings.HasPrefix(data, "pref:") {
		pref := strings.Split(data, ":")[1]
		user.Preference = pref
		_ = h.UserRepo.Update(user)
		h.sendUserProfile(chatID, user, true)

	} else if strings.HasPrefix(data, "mood:") {
		mood := strings.Split(data, ":")[1]
		user.CurrentMood = mood
		user.Status = "queue"
		user.PartnerID = 0
		_ = h.UserRepo.Update(user)
		
		cancelBtn := []telegram.InlineKeyboardButton{
			{Text: "❌ Cancel / Stop", CallbackData: "cmd:stop"},
		}
		cancelMarkup := telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{cancelBtn},
		}

		searchText := fmt.Sprintf(h.I18n.Get(user.LanguageCode, "joined_queue"), mood)
		searchText += "\n\n⏳ <i>Looking for a perfect match...</i>"
		
		_ = h.Bot.EditMessageText(chatID, msgID, searchText, cancelMarkup)
	}
}

func (h *BotHandler) sendRequest(req telegram.SendMessageRequest) {
	_, _ = h.Bot.SendMessageComplex(req)
}