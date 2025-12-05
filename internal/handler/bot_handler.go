package handler

import (
	"fmt"
	"log"
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
}

func NewBotHandler(bot *telegram.Client, userRepo *repository.UserRepository, i18n *i18n.I18nService) *BotHandler {
	return &BotHandler{
		Bot:      bot,
		UserRepo: userRepo,
		I18n:     i18n,
	}
}

func (h *BotHandler) HandleUpdate(update telegram.Update) {
	if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery)
		return
	}

	if update.Message != nil && update.Message.Text != "" {
		h.handleMessage(update.Message)
	}
}

func (h *BotHandler) handleMessage(msg *telegram.Message) {
	telegramID := msg.From.ID
	chatID := msg.Chat.ID
	
	user, err := h.UserRepo.GetByTelegramID(telegramID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	if user == nil {
		h.startOnboarding(msg)
		return
	}

	if msg.Text == "/stop" {
		h.stopChat(user)
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

	switch msg.Text {
	case "/start":
		// FIX: Parameter false, 0 (Pesan baru)
		h.sendMainMenu(chatID, user, false, 0)
		
	case "/profile":
		h.sendUserProfile(chatID, user, false)

	case "/search":
		h.cleanStatus(user)
		h.sendMoodSelector(chatID, user.LanguageCode, false, 0)

	case "/lang":
		h.sendLangSelector(chatID, user.LanguageCode, false, 0, "profile")

	case "/help":
		// FIX: Parameter false, 0 (Pesan baru)
		h.sendInfoMessage(chatID, user.LanguageCode, "help_text", false, 0)

	default:
		if user.Status == "queue" {
			_, _ = h.Bot.SendMessage(chatID, "Still searching... Type /stop to cancel.")
		} else {
			// FIX: Parameter false, 0 (Pesan baru)
			h.sendMainMenu(chatID, user, false, 0)
		}
	}
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
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_profile"), CallbackData: "cmd:profile"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_lang"), CallbackData: "edit:lang_from_menu"},
			},
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_help"), CallbackData: "cmd:help"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_about"), CallbackData: "cmd:about"},
			},
		},
	}

	// Gunakan fungsi helper sendOrEdit agar konsisten (Edit jika tombol back, Send jika /start)
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

	text := fmt.Sprintf(viewTemplate, user.FirstName, gender, pref, loc, statusText)

	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_gender"), CallbackData: "edit:gender"},
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_pref"), CallbackData: "edit:pref"},
			},
			{
				{Text: h.I18n.Get(user.LanguageCode, "btn_edit_loc"), CallbackData: "edit:loc"},
				// FIX: Callback spesifik
				{Text: h.I18n.Get(user.LanguageCode, "btn_lang"), CallbackData: "edit:lang_from_profile"},
			},
			// FIX: Tombol Back ke Menu Utama
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
	
	_, err := h.Bot.SendMessage(sender.PartnerID, msg.Text)
	if err != nil {
		log.Printf("Failed to relay message: %v", err)
		h.stopChat(sender)
	}
}

func (h *BotHandler) stopChat(initiator *core.User) {
	// 1. Idle -> Arahkan ke Mood Selector (Search)
	if initiator.Status == "idle" {
		// FIX: Redirect ke sendMoodSelector, bukan sendMainMenu
		h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)
		return
	}

	// 2. Queue -> Cancel & Arahkan ke Mood Selector (Search)
	if initiator.Status == "queue" {
		initiator.Status = "idle"
		initiator.PartnerID = 0 
		_ = h.UserRepo.Update(initiator)

		if initiator.LastMessageID != 0 {
			_ = h.Bot.EditMessageText(initiator.TelegramID, initiator.LastMessageID, "⛔ Search cancelled.", nil)
		} else {
			_, _ = h.Bot.SendMessage(initiator.TelegramID, "⛔ Search cancelled.")
		}
		
		// FIX: Redirect ke sendMoodSelector
		h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)
		return
	}

	// 3. Chatting -> End & Arahkan ke Mood Selector (Search)
	partnerID := initiator.PartnerID
	
	initiator.Status = "idle"
	initiator.PartnerID = 0
	_ = h.UserRepo.Update(initiator)
	
	_, _ = h.Bot.SendMessage(initiator.TelegramID, h.I18n.Get(initiator.LanguageCode, "chat_ended"))
	
	// FIX: Redirect ke sendMoodSelector
	h.sendMoodSelector(initiator.TelegramID, initiator.LanguageCode, false, 0)

	if partnerID != 0 {
		partner, err := h.UserRepo.GetByTelegramID(partnerID)
		if err == nil && partner != nil && partner.PartnerID == initiator.TelegramID {
			partner.Status = "idle"
			partner.PartnerID = 0
			_ = h.UserRepo.Update(partner)

			_, _ = h.Bot.SendMessage(partner.TelegramID, h.I18n.Get(partner.LanguageCode, "partner_left"))
			
			// FIX: Partner juga diredirect ke sendMoodSelector
			h.sendMoodSelector(partner.TelegramID, partner.LanguageCode, false, 0)
		}
	}
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

	// --- STOP/CANCEL ---
	if data == "cmd:stop" {
		h.stopChat(user)
		return
	}

	// --- NAVIGASI UTAMA (SEKARANG PAKE EDIT SEMUA) ---
	if data == "cmd:search" {
		h.cleanStatus(user)
		// Edit pesan menu menjadi mood selector
		h.sendMoodSelector(chatID, user.LanguageCode, true, msgID)
		return
	}
	if data == "cmd:profile" {
		// Edit pesan menu menjadi profile
		h.sendUserProfile(chatID, user, true)
		return
	}
	if data == "cmd:help" {
		// Edit pesan menu menjadi help
		h.sendInfoMessage(chatID, user.LanguageCode, "help_text", true, msgID)
		return
	}
	if data == "cmd:about" {
		// Edit pesan menu menjadi about
		h.sendInfoMessage(chatID, user.LanguageCode, "about_text", true, msgID)
		return
	}
	if data == "edit:lang_from_menu" {
		// Edit pesan menu menjadi lang selector
		h.sendLangSelector(chatID, user.LanguageCode, true, msgID, "menu")
		return
	}

	// --- NAVIGASI KEMBALI ---
	if data == "back:menu" {
		// Edit pesan apa pun kembali menjadi Menu Utama
		h.sendMainMenu(chatID, user, true, msgID)
		return
	}

	// --- PROFILE ACTIONS ---
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
			h.sendMainMenu(chatID, user, true, msgID)
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