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

	// 1. Cek Admin
	if strings.HasPrefix(msg.Text, "/") && h.Admin.IsAdmin(telegramID) {
		cmd := strings.Split(msg.Text, " ")[0]
		if cmd == "/stats" || cmd == "/broadcast" || cmd == "/addvip" {
			h.Admin.HandleCommand(msg)
			return 
		}
	}
	
	user, err := h.UserRepo.GetByTelegramID(telegramID)
	if err != nil { return }

	if user == nil {
		h.startOnboarding(msg)
		return
	}

	// --- [BARU] SATPAM PROFIL: Cek apakah data lengkap ---
	// Jika Gender atau Preferensi kosong, paksa user mengisi dulu.
	// Kecuali jika user sedang input lokasi (awaiting_location)
	if (user.Gender == "" || user.Preference == "") && user.Status != "awaiting_location" {
		if user.LastMessageID != 0 { _ = h.Bot.DeleteMessage(chatID, user.LastMessageID) }
		
		// FIX: Ambil teks dari i18n JSON
		warningMsg := h.I18n.Get(user.LanguageCode, "profile_incomplete")
		_, _ = h.Bot.SendMessage(chatID, warningMsg)
		
		h.sendGenderSelector(chatID, user.LanguageCode, false, 0)
		return
	}
	// -----------------------------------------------------

	if msg.Text == "/stop" {
		h.stopChat(user)
		return
	}

	if msg.Text == "/next" {
		h.handleNext(user)
		return
	}

	if msg.Text == "/share" {
		h.handleRevealRequest(user)
		return
	}

	if msg.Text == "/reconnect" {
		h.handleReconnect(user)
		return
	}

	if user.Status == "awaiting_location" {
		user.Location = msg.Text
		user.Status = "idle"
		_ = h.UserRepo.Update(user)
		
		confirmMsg := fmt.Sprintf(h.I18n.Get(user.LanguageCode, "location_saved"), user.Location)
		_, _ = h.Bot.SendMessage(chatID, confirmMsg)
		h.sendUserProfile(chatID, user, false)
		return
	}

	if user.Status == "chatting" {
		h.relayMessage(user, msg)
		return
	}

	// Bersihkan pesan lama saat command diketik
	if strings.HasPrefix(msg.Text, "/") && user.LastMessageID != 0 {
		_ = h.Bot.DeleteMessage(chatID, user.LastMessageID)
	}

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
	if loc == "" || loc == "-" {
		// Jika kosong, tampilkan ini
		loc = "🌍 Global / Not Set"
	}

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

	// --- [BARU] CHAT ACTION & SPOILER LOGIC ---
	
	var err error

	// 1. Jika FOTO
	if len(msg.Photo) > 0 {
		// Kirim action "uploading photo..."
		_ = h.Bot.SendChatAction(sender.PartnerID, "upload_photo")
		
		// Ambil foto kualitas tertinggi (terakhir di array)
		bestPhoto := msg.Photo[len(msg.Photo)-1]
		
		// Kirim Foto dengan SPOILER (Blur)
		req := telegram.SendPhotoRequest{
			ChatID:     sender.PartnerID,
			Photo:      bestPhoto.FileID, // Gunakan FileID dari Telegram
			Caption:    msg.Caption, // Caption jika ada
			HasSpoiler: true,     // AKTIFKAN BLUR
		}
		_, err = h.Bot.SendPhoto(req)

	// 2. Jika VIDEO
	} else if msg.Video != nil {
		_ = h.Bot.SendChatAction(sender.PartnerID, "upload_video")
		
		req := telegram.SendVideoRequest{
			ChatID:     sender.PartnerID,
			Video:      msg.Video.FileID,
			Caption:    msg.Caption,
			HasSpoiler: true, // AKTIFKAN BLUR
		}
		_, err = h.Bot.SendVideo(req)

	// 3. Jika VOICE NOTE
	} else if msg.Voice != nil {
		_ = h.Bot.SendChatAction(sender.PartnerID, "record_voice")
		// Voice tidak bisa di-spoiler, jadi copy biasa
		_, err = h.Bot.CopyMessage(sender.PartnerID, sender.TelegramID, msg.MessageID)

	// 4. Jika STIKER
	} else if msg.Sticker != nil {
		// Stiker tidak ada chat action khusus, kirim langsung
		_, err = h.Bot.CopyMessage(sender.PartnerID, sender.TelegramID, msg.MessageID)

	// 5. Jika TEKS BIASA
	} else {
		_ = h.Bot.SendChatAction(sender.PartnerID, "typing")
		_, err = h.Bot.CopyMessage(sender.PartnerID, sender.TelegramID, msg.MessageID)
	}
	
	// Error Handling
	if err != nil {
		log.Printf("Failed to relay message from %d to %d: %v", sender.TelegramID, sender.PartnerID, err)
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
		TelegramID:   msg.From.ID,
		Username:     msg.From.Username,
		FirstName:    msg.From.FirstName,
		LanguageCode: "en",
		Status:       "onboarding",
	}
	

	if err := h.UserRepo.Create(newUser); err != nil {
		log.Printf("Failed to create user: %v", err)
		return
	}

	// FIX: Jangan langsung menu utama! Kirim sapaan & tanya Gender.
	_, _ = h.Bot.SendMessage(msg.Chat.ID, h.I18n.Get(newUser.LanguageCode, "welcome"))
	h.sendGenderSelector(msg.Chat.ID, newUser.LanguageCode, false, 0)
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

	// --- [BARU] SATPAM PROFIL ---
	// Cek apakah aksi ini adalah aksi "Setup" (isi data)
	isSetupAction := strings.HasPrefix(data, "gender:") || 
					 strings.HasPrefix(data, "pref:") || 
					 strings.HasPrefix(data, "setlang:") ||
					 data == "edit:lang_from_menu" // Boleh ganti bahasa pas onboarding

	// Jika Gender/Pref kosong DAN user mencoba klik tombol fitur (bukan tombol setup)
	if (user.Gender == "" || user.Preference == "") && !isSetupAction {
		// Paksa kembali ke pemilihan Gender
		h.sendGenderSelector(chatID, user.LanguageCode, true, msgID)
		return
	}
	// ---------------------------

	if data == "reveal:agree" {
		// Hapus pesan permintaan agar tidak bisa diklik 2x
		_ = h.Bot.DeleteMessage(chatID, msgID)
		h.executeReveal(user)
		return
	}
	if data == "reveal:reject" {
		_ = h.Bot.DeleteMessage(chatID, msgID)
		_, _ = h.Bot.SendMessage(chatID, h.I18n.Get(user.LanguageCode, "share_rejected"))
		return
	}

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

	// Pembayaran
	if strings.HasPrefix(data, "buy:") {
		planID := strings.TrimPrefix(data, "buy:")
		h.Payment.SendVIPInvoice(chatID, planID, user.LanguageCode)
		return
	}

	// --- NAVIGASI MENU UTAMA ---
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
	
	// Sub-menu Help
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

	// --- NAVIGASI EDIT/SETTING ---
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

	// --- SAVING DATA ---
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
		
		// Jika ini bagian dari onboarding (status masih onboarding/kosong)
		if user.Status == "onboarding" || user.Preference == "" {
			h.sendPreferenceSelector(chatID, user.LanguageCode, true, msgID)
		} else {
			h.sendUserProfile(chatID, user, true)
		}

	} else if strings.HasPrefix(data, "pref:") {
		pref := strings.Split(data, ":")[1]
		user.Preference = pref
		_ = h.UserRepo.Update(user)
		
		// Jika selesai onboarding, arahkan ke Menu Utama
		if user.Status == "onboarding" {
			// Update status biar ga dianggap onboarding lagi
			user.Status = "idle"
			_ = h.UserRepo.Update(user)
			
			_, _ = h.Bot.SendMessage(chatID, h.I18n.Get(user.LanguageCode, "setup_complete"))
			
			// Hapus selector lama, kirim menu utama baru
			_ = h.Bot.DeleteMessage(chatID, msgID)
			h.sendMainMenu(chatID, user, false, 0)
		} else {
			h.sendUserProfile(chatID, user, true)
		}

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

// [BARU] Fungsi Meminta Izin Reveal
func (h *BotHandler) handleRevealRequest(sender *core.User) {
	// 1. Cek apakah sedang chatting
	if sender.Status != "chatting" || sender.PartnerID == 0 {
		_, _ = h.Bot.SendMessage(sender.TelegramID, "⚠️ You are not in a chat.")
		return
	}

	// 2. Cek apakah pengirim punya username
	if sender.Username == "" {
		_, _ = h.Bot.SendMessage(sender.TelegramID, h.I18n.Get(sender.LanguageCode, "share_error_no_username"))
		return
	}

	// 3. Kirim Konfirmasi ke Pengirim
	_, _ = h.Bot.SendMessage(sender.TelegramID, h.I18n.Get(sender.LanguageCode, "share_request_sent"))

	// 4. Kirim Permintaan ke Partner
	partner, err := h.UserRepo.GetByTelegramID(sender.PartnerID)
	if err != nil || partner == nil { return }

	msgText := h.I18n.Get(partner.LanguageCode, "share_request_received")
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ Accept", CallbackData: "reveal:agree"},
				{Text: "❌ Reject", CallbackData: "reveal:reject"},
			},
		},
	}
	_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: partner.TelegramID, Text: msgText, ReplyMarkup: keyboard, ParseMode: "HTML",
	})
}

// [BARU] Fungsi Eksekusi Tukar Kontak
func (h *BotHandler) executeReveal(accepter *core.User) {
	// Accepter adalah orang yang mengklik "Accept"
	
	// 1. Cek validitas chat
	if accepter.Status != "chatting" || accepter.PartnerID == 0 {
		return
	}

	requester, err := h.UserRepo.GetByTelegramID(accepter.PartnerID)
	if err != nil || requester == nil { return }

	// 2. Cek Username (Double Check)
	if accepter.Username == "" || requester.Username == "" {
		errMsg := h.I18n.Get(accepter.LanguageCode, "share_error_no_username")
		_, _ = h.Bot.SendMessage(accepter.TelegramID, errMsg)
		_, _ = h.Bot.SendMessage(requester.TelegramID, errMsg)
		return
	}

	// 3. Kirim Kontak Requester ke Accepter
	msgToAccepter := fmt.Sprintf(h.I18n.Get(accepter.LanguageCode, "share_accepted_us"), requester.FirstName, requester.Username)
	_, _ = h.Bot.SendMessage(accepter.TelegramID, msgToAccepter)

	// 4. Kirim Kontak Accepter ke Requester
	msgToRequester := fmt.Sprintf(h.I18n.Get(requester.LanguageCode, "share_accepted_us"), accepter.FirstName, accepter.Username)
	_, _ = h.Bot.SendMessage(requester.TelegramID, msgToRequester)
}

func (h *BotHandler) handleNext(initiator *core.User) {
	// 1. Jika User IDLE (Gak ngapa-ngapain), arahkan ke Search
	if initiator.Status == "idle" {
		h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)
		return
	}

	// 2. Jika User QUEUE (Sedang cari), refresh pencarian saja
	if initiator.Status == "queue" {
		mood := initiator.CurrentMood
		cancelBtn := []telegram.InlineKeyboardButton{
			{Text: "❌ Cancel / Stop", CallbackData: "cmd:stop"},
		}
		cancelMarkup := telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{cancelBtn},
		}
		
		searchText := fmt.Sprintf("⏭ <b>Skipping...</b>\n" + h.I18n.Get(initiator.LanguageCode, "joined_queue"), mood)
		searchText += "\n\n⏳ <i>Looking for a perfect match...</i>"

		if initiator.LastMessageID != 0 {
			_ = h.Bot.EditMessageText(initiator.TelegramID, initiator.LastMessageID, searchText, cancelMarkup)
		} else {
			msgID, _ := h.Bot.SendMessageComplex(telegram.SendMessageRequest{
				ChatID: initiator.TelegramID, Text: searchText, ReplyMarkup: cancelMarkup, ParseMode: "HTML",
			})
			if msgID != 0 {
				initiator.LastMessageID = msgID
				_ = h.UserRepo.Update(initiator)
			}
		}
		return
	}

	// 3. Jika User CHATTING
	partnerID := initiator.PartnerID
	currentMood := initiator.CurrentMood

	// A. Update Initiator (Pelaku Next) -> Langsung masuk QUEUE
	initiator.LastPartnerID = partnerID
	initiator.Status = "queue" // Langsung antri lagi
	initiator.PartnerID = 0
	initiator.CurrentMood = currentMood // Pastikan mood tetap sama
	_ = h.UserRepo.Update(initiator)

	// Tampilkan Animasi Searching ke Initiator
	cancelBtn := []telegram.InlineKeyboardButton{
		{Text: "❌ Cancel / Stop", CallbackData: "cmd:stop"},
	}
	cancelMarkup := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{cancelBtn},
	}
	
	searchText := fmt.Sprintf("⏭ <b>Skipping...</b>\n" + h.I18n.Get(initiator.LanguageCode, "joined_queue"), currentMood)
	searchText += "\n\n⏳ <i>Looking for a new partner...</i>"

	_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: initiator.TelegramID, Text: searchText, ReplyMarkup: cancelMarkup, ParseMode: "HTML",
	})

	// B. Update Partner (Korban yang di-skip) -> Jadi IDLE
	if partnerID != 0 {
		partner, err := h.UserRepo.GetByTelegramID(partnerID)
		if err == nil && partner != nil && partner.PartnerID == initiator.TelegramID {
			
			partner.LastPartnerID = initiator.TelegramID
			partner.Status = "idle"
			partner.PartnerID = 0
			_ = h.UserRepo.Update(partner)

			// Beritahu partner kalau dia ditinggal
			stopTextPartner := h.I18n.Get(partner.LanguageCode, "partner_left")
			
			// Tawarkan Reconnect (Upselling VIP) ke Partner
			reconnectBtnPartner := telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: h.I18n.Get(partner.LanguageCode, "btn_reconnect"), CallbackData: "cmd:reconnect_teaser"}},
				},
			}
			_, _ = h.Bot.SendMessageComplex(telegram.SendMessageRequest{
				ChatID: partner.TelegramID, Text: stopTextPartner, ReplyMarkup: reconnectBtnPartner, ParseMode: "HTML",
			})

			// Kembalikan partner ke menu mood
			h.sendMoodSelector(partner.TelegramID, partner.LanguageCode, false, 0)
		}
	}
}