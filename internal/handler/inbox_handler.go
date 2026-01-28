package handler

import (
	"fmt"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"log"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"strconv"
	"strings"
)

const InviteBannerURL = "https://i.ibb.co.com/C5SFnyx8/Untitled-design-3.png"

type InboxHandler struct {
	Bot       *telegram.Client
	InboxRepo *repository.InboxRepository
	UserRepo  *repository.UserRepository
	I18n      *i18n.I18nService
}

func NewInboxHandler(bot *telegram.Client, inboxRepo *repository.InboxRepository, userRepo *repository.UserRepository, i18n *i18n.I18nService) *InboxHandler {
	return &InboxHandler{
		Bot:       bot,
		InboxRepo: inboxRepo,
		UserRepo:  userRepo,
		I18n:      i18n,
	}
}

func (h *InboxHandler) HandleInlineQuery(query *telegram.InlineQuery) {
	// Ubah versi cache ke v3 agar memaksa refresh tampilan di HP
	resultID := fmt.Sprintf("%d_v3", query.From.ID)
	deepLink := fmt.Sprintf("https://t.me/%s?start=secret_%d", h.Bot.GetBotUsername(), query.From.ID)

	// [LOGIKA BARU] Prioritas Bahasa: Database > HP > Default
	var lang string

	// 1. Cek Database dulu (Apakah user sudah set /lang?)
	user, err := h.UserRepo.GetByTelegramID(query.From.ID)
	if err == nil && user != nil {
		lang = user.LanguageCode
		log.Printf("👤 [INLINE] User ditemukan di DB. Menggunakan Bahasa DB: '%s'", lang)
	} else {
		// 2. Kalau user baru/belum ada di DB, pakai bahasa HP
		lang = query.From.LanguageCode
		log.Printf("📱 [INLINE] User baru/Gagal DB. Menggunakan Bahasa HP: '%s'", lang)
	}

	// Normalisasi (jika formatnya 'en-US' ambil 'en' saja)
	if len(lang) > 2 {
		lang = lang[:2]
	}
	// Fallback jika kosong
	if lang == "" {
		lang = "en"
	}
	
	// Ambil Teks sesuai bahasa yang sudah ditentukan
	btnText := h.I18n.Get(lang, "inbox_btn_invite")
	title := h.I18n.Get(lang, "inbox_inline_title")
	desc := h.I18n.Get(lang, "inbox_inline_desc")

	textParams := fmt.Sprintf("<a href=\"%s\">&#8205;</a>%s", InviteBannerURL, "Click the button below to send me a secret message! 🤫")
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: btnText, Url: deepLink}},
		},
	}

	article := telegram.InlineQueryResultArticle{
		Type:  "article",
		ID:    resultID,
		Title: title,       
		Description: desc,  
		InputMessageContent: telegram.InputMessageContent{
			MessageText: textParams,
			ParseMode:   "HTML",
		},
		ReplyMarkup: &keyboard,
		ThumbURL:    InviteBannerURL, 
	}

	results := []interface{}{article}
	h.Bot.AnswerInlineQuery(query.ID, results)
}

func (h *InboxHandler) HandleIncomingSecretMessage(sender *core.User, text string) {
	targetID := sender.LastPartnerID 

	msg := &core.InboxMessage{
		ReceiverID: targetID,
		SenderID:   sender.TelegramID,
		Message:    text,
	}

	if err := h.InboxRepo.SaveMessage(msg); err != nil {
		h.Bot.SendMessage(sender.TelegramID, "❌ System Error.")
		return
	}

	h.Bot.SendMessage(sender.TelegramID, h.I18n.Get(sender.LanguageCode, "secret_sent_success"))

	sender.Status = "idle"
	sender.LastPartnerID = 0 
	h.UserRepo.Update(sender)

	h.notifyReceiver(targetID)
}

func (h *InboxHandler) notifyReceiver(targetID int64) {
	target, err := h.UserRepo.GetByTelegramID(targetID)
	if err != nil || target == nil { return }

	notifText := h.I18n.Get(target.LanguageCode, "secret_received")
	h.Bot.SendMessage(targetID, notifText)
}

func (h *InboxHandler) ShowInbox(user *core.User) {
	messages, err := h.InboxRepo.GetMessagesByReceiver(user.TelegramID)
	if err != nil {
		h.Bot.SendMessage(user.TelegramID, "❌ Error fetching inbox.")
		return
	}

	lang := user.LanguageCode

	if len(messages) == 0 {
		h.Bot.SendMessage(user.TelegramID, h.I18n.Get(lang, "inbox_empty"))
		return
	}

	// 1. Kirim Header
	header := fmt.Sprintf(h.I18n.Get(lang, "inbox_header"), len(messages))
	h.Bot.SendMessage(user.TelegramID, header)

	// 2. Loop dan kirim pesan satu per satu dengan tombol PEEK
	for _, msg := range messages {
		// Escape HTML
		safeMsg := strings.ReplaceAll(msg.Message, "<", "&lt;")
		safeMsg = strings.ReplaceAll(safeMsg, ">", "&gt;")
		
		formattedMsg := fmt.Sprintf("<blockquote>%s</blockquote>", safeMsg)

		// Tombol Intip (Callback: peek:MSG_ID)
		keyboard := telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{
				{{Text: h.I18n.Get(lang, "btn_peek"), CallbackData: fmt.Sprintf("peek:%d", msg.ID)}},
			},
		}

		h.Bot.SendMessageWithMarkup(user.TelegramID, formattedMsg, keyboard)
	}

	// 3. Tombol Bersihkan Inbox (Paling Bawah)
	clearKeyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: h.I18n.Get(lang, "btn_clear_inbox"), CallbackData: "clear_inbox"}},
		},
	}
	h.Bot.SendMessageWithMarkup(user.TelegramID, "👇", clearKeyboard)
}

func (h *InboxHandler) HandlePeek(cb *telegram.CallbackQuery, user *core.User) {
	parts := strings.Split(cb.Data, ":")
	if len(parts) < 2 { return }
	
	msgIDStr := parts[1]
	msgID, _ := strconv.ParseInt(msgIDStr, 10, 64)

	// Helper lokal: Bersihkan tag HTML karena Popup Telegram tidak support formatting
	stripHTML := func(s string) string {
		s = strings.ReplaceAll(s, "<b>", "")
		s = strings.ReplaceAll(s, "</b>", "")
		s = strings.ReplaceAll(s, "<i>", "")
		s = strings.ReplaceAll(s, "</i>", "")
		s = strings.ReplaceAll(s, "<code>", "")
		s = strings.ReplaceAll(s, "</code>", "")
		return s
	}

	// 1. Cek VIP
	if !user.IsVIP {
		alertText := h.I18n.Get(user.LanguageCode, "peek_locked")
		// [PERBAIKAN] Bersihkan HTML sebelum kirim ke Popup
		h.Bot.AnswerCallbackQuery(cb.ID, stripHTML(alertText), true) 
		return
	}

	// 2. Ambil Pesan
	msg, err := h.InboxRepo.GetMessageByID(msgID)
	if err != nil || msg == nil {
		h.Bot.AnswerCallbackQuery(cb.ID, "❌ Message not found.", false)
		return
	}

	// 3. Ambil Info Pengirim
	sender, err := h.UserRepo.GetByTelegramID(msg.SenderID)
	if err != nil || sender == nil {
		h.Bot.AnswerCallbackQuery(cb.ID, "❌ Sender not found.", false)
		return
	}

	// 4. Generate Clue (Masking Nama)
	maskedName := "Unknown"
	if len(sender.FirstName) > 0 {
		runes := []rune(sender.FirstName)
		maskedName = string(runes[0]) + "***"
	}
	
	genderText := h.I18n.Get(user.LanguageCode, "gender_unknown")
	if sender.Gender == "male" {
		genderText = h.I18n.Get(user.LanguageCode, "gender_male")
	} else if sender.Gender == "female" {
		genderText = h.I18n.Get(user.LanguageCode, "gender_female")
	}

	// 5. Tampilkan Popup Clue
	clueText := fmt.Sprintf(h.I18n.Get(user.LanguageCode, "peek_result"), genderText, maskedName)
	
	// [PERBAIKAN] Bersihkan HTML sebelum kirim ke Popup
	h.Bot.AnswerCallbackQuery(cb.ID, stripHTML(clueText), true)
}

func (h *InboxHandler) HandleClear(cb *telegram.CallbackQuery, user *core.User) {
	_ = h.InboxRepo.DeleteMessagesByReceiver(user.TelegramID)
	
	confirmText := h.I18n.Get(user.LanguageCode, "inbox_cleared")
	// PERBAIKAN: Hapus "_ ="
	h.Bot.AnswerCallbackQuery(cb.ID, confirmText, true)
	
	h.Bot.EditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, confirmText, nil)
}

// [PERBARUAN] 1. Tahap Tanya: Ubah tombol hapus jadi pertanyaan konfirmasi
func (h *InboxHandler) HandleAskClear(cb *telegram.CallbackQuery, user *core.User) {
	// Teks konfirmasi
	text := h.I18n.Get(user.LanguageCode, "inbox_confirm_text")
	
	// Tombol Ya / Tidak
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_yes"), CallbackData: "clear_yes"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_no"), CallbackData: "clear_no"},
			},
		},
	}

	// Edit pesan tombol tadi menjadi pesan konfirmasi
	h.Bot.EditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text, &keyboard)
}

// [PERBARUAN] 2. Tahap Eksekusi: Hapus data jika user klik YA
func (h *InboxHandler) HandleConfirmClear(cb *telegram.CallbackQuery, user *core.User) {
	// Hapus pesan di database
	_ = h.InboxRepo.DeleteMessagesByReceiver(user.TelegramID)
	
	confirmText := h.I18n.Get(user.LanguageCode, "inbox_cleared")
	
	// Tampilkan Popup Sukses
	h.Bot.AnswerCallbackQuery(cb.ID, confirmText, true)
	
	// Ubah pesan konfirmasi jadi status "Telah Dihapus" (Hilangkan tombol)
	h.Bot.EditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, confirmText, nil)
}

// [PERBARUAN] 3. Tahap Batal: Kembalikan tombol seperti semula jika user klik TIDAK
func (h *InboxHandler) HandleCancelClear(cb *telegram.CallbackQuery, user *core.User) {
	// Kembalikan ke tampilan tombol awal (👇 + Tombol Clear)
	initialText := "👇"
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: h.I18n.Get(user.LanguageCode, "btn_clear_inbox"), CallbackData: "clear_inbox"}},
		},
	}

	h.Bot.EditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, initialText, &keyboard)
}