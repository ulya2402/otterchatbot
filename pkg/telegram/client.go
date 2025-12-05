package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const telegramAPIBase = "https://api.telegram.org/bot"

type Client struct {
	Token      string
	HttpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		Token: token,
		HttpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) GetUpdates(offset int) ([]Update, error) {
	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=10", telegramAPIBase, c.Token, offset)
	resp, err := c.HttpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updates: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse json response: %v", err)
	}

	if !apiResp.Ok {
		return nil, fmt.Errorf("telegram api error: %d %s", apiResp.ErrorCode, apiResp.Description)
	}

	return apiResp.Result, nil
}

func (c *Client) SendChatAction(chatID int64, action string) error {
	req := SendChatActionRequest{
		ChatID: chatID,
		Action: action,
	}
	jsonData, _ := json.Marshal(req)
	url := fmt.Sprintf("%s%s/sendChatAction", telegramAPIBase, c.Token)
	_, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	return err
}

func (c *Client) SendMessage(chatID int64, text string) (int, error) {
	req := SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	return c.SendMessageComplex(req)
}

func (c *Client) SendMessageComplex(req SendMessageRequest) (int, error) {
	if req.ParseMode == "" {
		req.ParseMode = "HTML"
	}
	
	jsonData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal error: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendMessage", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	var apiResp struct {
		Ok          bool   `json:"ok"`
		Result      Message `json:"result"`
		Description string `json:"description"`
	}
	
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, nil 
	}

	if !apiResp.Ok {
		return 0, fmt.Errorf("api error: %s", apiResp.Description)
	}

	return apiResp.Result.MessageID, nil
}

// BARU: Fungsi untuk mengirim foto via URL
func (c *Client) SendPhoto(req SendPhotoRequest) (int, error) {
	if req.ParseMode == "" {
		req.ParseMode = "HTML"
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal error: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendPhoto", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp struct {
		Ok          bool   `json:"ok"`
		Result      Message `json:"result"`
		Description string `json:"description"`
	}
	
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, nil
	}

	if !apiResp.Ok {
		return 0, fmt.Errorf("api error: %s", apiResp.Description)
	}

	return apiResp.Result.MessageID, nil
}

func (c *Client) SendVideo(req SendVideoRequest) (int, error) {
	if req.ParseMode == "" {
		req.ParseMode = "HTML"
	}
	jsonData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal error: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendVideo", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp struct {
		Ok          bool    `json:"ok"`
		Result      Message `json:"result"`
		Description string  `json:"description"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, nil
	}
	if !apiResp.Ok {
		return 0, fmt.Errorf("api error: %s", apiResp.Description)
	}
	return apiResp.Result.MessageID, nil
}

func (c *Client) SendInvoice(req SendInvoiceRequest) error {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal invoice: %v", err)
	}

	url := fmt.Sprintf("%s%s/sendInvoice", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send invoice: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error: %s", string(body))
	}
	return nil
}

// [BARU] Fungsi Jawab PreCheckout (Wajib untuk Payments)
func (c *Client) AnswerPreCheckoutQuery(queryID string, ok bool, errorMessage string) error {
	req := AnswerPreCheckoutQueryRequest{
		PreCheckoutQueryID: queryID,
		Ok:                 ok,
		ErrorMessage:       errorMessage,
	}
	jsonData, _ := json.Marshal(req)

	url := fmt.Sprintf("%s%s/answerPreCheckoutQuery", telegramAPIBase, c.Token)
	_, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	return err
}

func (c *Client) EditMessageText(chatID int64, messageID int, text string, replyMarkup interface{}) error {
	req := struct {
		ChatID      int64       `json:"chat_id"`
		MessageID   int         `json:"message_id"`
		Text        string      `json:"text"`
		ParseMode   string      `json:"parse_mode"`
		ReplyMarkup interface{} `json:"reply_markup,omitempty"`
	}{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: replyMarkup,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s/editMessageText", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) DeleteMessage(chatID int64, messageID int) error {
	req := struct {
		ChatID    int64 `json:"chat_id"`
		MessageID int   `json:"message_id"`
	}{
		ChatID:    chatID,
		MessageID: messageID,
	}

	jsonData, _ := json.Marshal(req)
	url := fmt.Sprintf("%s%s/deleteMessage", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) AnswerCallbackQuery(callbackQueryID string, text string) {
	req := struct {
		CallbackQueryID string `json:"callback_query_id"`
		Text            string `json:"text,omitempty"`
	}{
		CallbackQueryID: callbackQueryID,
		Text:            text,
	}
	jsonData, _ := json.Marshal(req)
	url := fmt.Sprintf("%s%s/answerCallbackQuery", telegramAPIBase, c.Token)
	_, _ = c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
}

func (c *Client) CopyMessage(toChatID int64, fromChatID int64, messageID int) (int, error) {
	req := CopyMessageRequest{
		ChatID:     toChatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal error: %v", err)
	}

	url := fmt.Sprintf("%s%s/copyMessage", telegramAPIBase, c.Token)
	resp, err := c.HttpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("request error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp struct {
		Ok          bool    `json:"ok"`
		Result      struct{ MessageID int `json:"message_id"` } `json:"result"`
		Description string  `json:"description"`
	}
	
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, nil
	}

	if !apiResp.Ok {
		return 0, fmt.Errorf("api error: %s", apiResp.Description)
	}

	return apiResp.Result.MessageID, nil
}