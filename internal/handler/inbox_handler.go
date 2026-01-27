package handler

import (
	"fmt"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
)

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
	resultID := fmt.Sprintf("%d", query.From.ID)
	deepLink := fmt.Sprintf("https://t.me/%s?start=secret_%d", h.Bot.GetBotUsername(), query.From.ID)

	lang := query.From.LanguageCode
	btnText := h.I18n.Get(lang, "inbox_btn_invite")
	title := h.I18n.Get(lang, "inbox_article_title")
	desc := h.I18n.Get(lang, "inbox_article_desc")

	textParams := "Click the button below to send me a secret message! 🤫"
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: btnText, Url: deepLink}},
		},
	}

	article := telegram.InlineQueryResultArticle{
		Type:  "article",
		ID:    resultID,
		Title: title,
		InputMessageContent: telegram.InputMessageContent{
			MessageText: textParams,
			ParseMode:   "HTML",
		},
		ReplyMarkup: &keyboard,
		Description: desc,
		ThumbURL:    "https://img.icons8.com/color/48/shh.png", 
	}

	results := []interface{}{article}
	_ = h.Bot.AnswerInlineQuery(query.ID, results)
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

	header := fmt.Sprintf(h.I18n.Get(lang, "inbox_header"), len(messages))
	text := header
	
	for _, msg := range messages {
		// Fungsi escapeHTML akan otomatis menggunakan yang ada di bot_handler.go karena satu package
		safeMsg := escapeHTML(msg.Message)
		text += fmt.Sprintf("<blockquote>%s</blockquote>\n\n", safeMsg)
	}

	text += h.I18n.Get(lang, "inbox_footer")

	h.Bot.SendMessage(user.TelegramID, text)

	_ = h.InboxRepo.DeleteMessagesByReceiver(user.TelegramID)
}