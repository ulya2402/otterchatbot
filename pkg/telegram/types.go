package telegram

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
	PreCheckoutQuery   *PreCheckoutQuery `json:"pre_checkout_query"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
	Caption            string             `json:"caption"`
	SuccessfulPayment  *SuccessfulPayment `json:"successful_payment"`
	Photo              []PhotoSize        `json:"photo"`
	Video              *Video             `json:"video"`
	Voice              *Voice             `json:"voice"`
	Sticker            *Sticker           `json:"sticker"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size"`
}

// [BARU] Struct Video
type Video struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
}

// [BARU] Struct Voice
type Voice struct {
	FileID string `json:"file_id"`
}

// [BARU] Struct Sticker
type Sticker struct {
	FileID string `json:"file_id"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type PreCheckoutQuery struct {
	ID             string `json:"id"`
	From           *User  `json:"from"`
	Currency       string `json:"currency"`
	TotalAmount    int    `json:"total_amount"`
	InvoicePayload string `json:"invoice_payload"`
}

type SuccessfulPayment struct {
	Currency                string `json:"currency"`
	TotalAmount             int    `json:"total_amount"`
	InvoicePayload          string `json:"invoice_payload"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id"`
}

// [BARU] Harga Label
type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int    `json:"amount"`
}

type SendMessageRequest struct {
	ChatID      int64       `json:"chat_id"`
	Text        string      `json:"text"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

type SendPhotoRequest struct {
	ChatID      int64       `json:"chat_id"`
	Photo       string      `json:"photo"`
	Caption     string      `json:"caption,omitempty"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	HasSpoiler  bool        `json:"has_spoiler,omitempty"` // Efek Blur
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

type SendVideoRequest struct {
	ChatID      int64       `json:"chat_id"`
	Video       string      `json:"video"`
	Caption     string      `json:"caption,omitempty"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	HasSpoiler  bool        `json:"has_spoiler,omitempty"` // Efek Blur
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

type SendChatActionRequest struct {
	ChatID int64  `json:"chat_id"`
	Action string `json:"action"` // typing, upload_photo, etc
}

type SendInvoiceRequest struct {
	ChatID        int64          `json:"chat_id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Payload       string         `json:"payload"`
	Currency      string         `json:"currency"`
	Prices        []LabeledPrice `json:"prices"`
	ProviderToken string         `json:"provider_token"` // Kosong untuk Stars
}

// [BARU] Request untuk menjawab Pre-Checkout
type AnswerPreCheckoutQueryRequest struct {
	PreCheckoutQueryID string `json:"pre_checkout_query_id"`
	Ok                 bool   `json:"ok"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

type APIResponse struct {
	Ok          bool            `json:"ok"`
	Result      []Update        `json:"result"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	Url          string `json:"url,omitempty"`
	Pay          bool   `json:"pay,omitempty"`
}

type CopyMessageRequest struct {
	ChatID     int64 `json:"chat_id"`      // Ke mana pesan dikirim
	FromChatID int64 `json:"from_chat_id"` // Dari mana pesan berasal
	MessageID  int   `json:"message_id"`   // ID Pesan yang mau dikopi
}

// [PEMBARUAN 6] Struktur untuk SetMyCommands
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type BotCommandScope struct {
	Type string `json:"type"` // default, all_private_chats, dll
}

type SetMyCommandsRequest struct {
	Commands     []BotCommand     `json:"commands"`
	Scope        *BotCommandScope `json:"scope,omitempty"`
	LanguageCode string           `json:"language_code,omitempty"`
}