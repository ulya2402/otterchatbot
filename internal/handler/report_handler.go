package handler

import (
	"fmt"
	"log"
	"otterchatbot/config"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"strconv"
	"strings"
)

type ReportHandler struct {
	Bot      *telegram.Client
	UserRepo *repository.UserRepository
	Config   *config.Config
	I18n     *i18n.I18nService
}

func NewReportHandler(bot *telegram.Client, repo *repository.UserRepository, cfg *config.Config, i18n *i18n.I18nService) *ReportHandler {
	return &ReportHandler{
		Bot:      bot,
		UserRepo: repo,
		Config:   cfg,
		I18n:     i18n,
	}
}

// [PEMBARUAN 5] Menggunakan i18n (Multi-bahasa)
func (h *ReportHandler) HandleReportCommand(reporter *core.User) {
	if reporter.PartnerID == 0 {
		h.Bot.SendMessage(reporter.TelegramID, h.I18n.Get(reporter.LanguageCode, "report_error_no_chat"))
		return
	}

	text := h.I18n.Get(reporter.LanguageCode, "report_menu_title")
	
	keyboard := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: h.I18n.Get(reporter.LanguageCode, "report_reason_porn"), CallbackData: "report:porn"}},
			{{Text: h.I18n.Get(reporter.LanguageCode, "report_reason_harass"), CallbackData: "report:harass"}},
			{{Text: h.I18n.Get(reporter.LanguageCode, "report_reason_spam"), CallbackData: "report:spam"}},
			{{Text: h.I18n.Get(reporter.LanguageCode, "report_reason_scam"), CallbackData: "report:scam"}},
			{{Text: h.I18n.Get(reporter.LanguageCode, "report_btn_cancel"), CallbackData: "cmd:delete_me"}},
		},
	}

	h.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: reporter.TelegramID, Text: text, ReplyMarkup: keyboard, ParseMode: "HTML",
	})
}

func (h *ReportHandler) HandleReportCallback(reporter *core.User, reasonCode string) {
	targetID := reporter.PartnerID
	if targetID == 0 {
		targetID = reporter.LastPartnerID
	}

	if targetID == 0 {
		h.Bot.SendMessage(reporter.TelegramID, h.I18n.Get(reporter.LanguageCode, "report_error_generic"))
		return
	}

	targetUser, err := h.UserRepo.GetByTelegramID(targetID)
	if err != nil || targetUser == nil {
		h.Bot.SendMessage(reporter.TelegramID, h.I18n.Get(reporter.LanguageCode, "report_error_generic"))
		return
	}

	// Mapping alasan untuk Admin (Tetap Inggris agar Admin paham)
	reasonMap := map[string]string{
		"porn": "🔞 Pornography",
		"harass": "🤬 Harassment",
		"spam": "📢 Spam",
		"scam": "👺 Scam",
	}
	reasonText := reasonMap[reasonCode]
	if reasonText == "" { reasonText = "Other" }

	// A. Beritahu Reporter (Sesuai bahasa Reporter)
	h.Bot.SendMessage(reporter.TelegramID, h.I18n.Get(reporter.LanguageCode, "report_sent"))

	// B. Kirim ke Semua Admin
	for _, adminIDStr := range h.Config.AdminIDs {
		adminID, _ := strconv.ParseInt(adminIDStr, 10, 64)
		if adminID == 0 { continue }

		reportCard := fmt.Sprintf(
			"🚨 <b>NEW REPORT RECEIVED</b>\n\n"+
			"👤 <b>Reporter:</b> %s (ID: <code>%d</code>)\n"+
			"🚫 <b>Accused:</b> %s (ID: <code>%d</code>)\n"+
			"📛 <b>Username:</b> @%s\n"+
			"📝 <b>Reason:</b> %s\n\n"+
			"<i>Action needed:</i>",
			reporter.FirstName, reporter.TelegramID,
			targetUser.FirstName, targetUser.TelegramID,
			targetUser.Username,
			reasonText,
		)

		actions := telegram.InlineKeyboardMarkup{
			InlineKeyboard: [][]telegram.InlineKeyboardButton{
				{
					{Text: "🚫 BAN USER", CallbackData: fmt.Sprintf("admin:ban:%d", targetUser.TelegramID)},
					{Text: "⚠️ WARN", CallbackData: fmt.Sprintf("admin:warn:%d", targetUser.TelegramID)},
				},
				{
					{Text: "✅ DISMISS", CallbackData: "admin:dismiss"},
				},
			},
		}

		h.Bot.SendMessageComplex(telegram.SendMessageRequest{
			ChatID: adminID, Text: reportCard, ReplyMarkup: actions, ParseMode: "HTML",
		})
	}
}

func (h *ReportHandler) HandleAdminAction(adminID int64, data string, msgID int) {
	parts := strings.Split(data, ":")
	action := parts[1] // ban, warn, dismiss

	if action == "dismiss" {
		h.Bot.EditMessageText(adminID, msgID, "✅ <b>Report Dismissed.</b> No action taken.", nil)
		return
	}

	if len(parts) < 3 { return }
	targetIDStr := parts[2]
	targetID, _ := strconv.ParseInt(targetIDStr, 10, 64)

	targetUser, err := h.UserRepo.GetByTelegramID(targetID)
	if err != nil || targetUser == nil {
		h.Bot.SendMessage(adminID, "❌ User not found.")
		return
	}

	if action == "ban" {
		targetUser.IsBanned = true
		targetUser.Status = "banned"
		targetUser.PartnerID = 0
		_ = h.UserRepo.Update(targetUser)

		// [PEMBARUAN 5] Kirim notifikasi sesuai bahasa Target User
		h.Bot.SendMessage(targetID, h.I18n.Get(targetUser.LanguageCode, "ban_notification"))

		h.Bot.EditMessageText(adminID, msgID, fmt.Sprintf("🚫 <b>BANNED!</b>\nUser %s has been banned.", targetUser.FirstName), nil)
		log.Printf("User %d BANNED by Admin %d", targetID, adminID)

	} else if action == "warn" {
		// [PEMBARUAN 5] Kirim peringatan sesuai bahasa Target User
		h.Bot.SendMessage(targetID, h.I18n.Get(targetUser.LanguageCode, "warn_notification"))
		
		h.Bot.EditMessageText(adminID, msgID, fmt.Sprintf("⚠️ <b>Warned!</b>\nWarning sent to %s.", targetUser.FirstName), nil)
	}
}