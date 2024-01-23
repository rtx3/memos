package integration

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"unicode/utf16"

	"github.com/lithammer/shortuuid/v4"
	"github.com/pkg/errors"

	apiv1 "github.com/usememos/memos/api/v1"
	"github.com/usememos/memos/plugin/telegram"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

type TelegramHandler struct {
	store *store.Store
}

func NewTelegramHandler(store *store.Store) *TelegramHandler {
	return &TelegramHandler{store: store}
}

func (t *TelegramHandler) BotToken(ctx context.Context) string {
	return t.store.GetSystemSettingValueWithDefault(ctx, apiv1.SystemSettingTelegramBotTokenName.String(), "")
}

const (
	workingMessage = "Working on sending your memo..."
	successMessage = "Success"
)

func (t *TelegramHandler) MessageHandle(ctx context.Context, bot *telegram.Bot, message telegram.Message, attachments []telegram.Attachment) error {
	reply, err := bot.SendReplyMessage(ctx, message.Chat.ID, message.MessageID, workingMessage)
	if err != nil {
		return errors.Wrap(err, "Failed to SendReplyMessage")
	}

	var creatorID int32
	userSettingList, err := t.store.ListUserSettings(ctx, &store.FindUserSetting{
		Key: storepb.UserSettingKey_USER_SETTING_TELEGRAM_USER_ID,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to find userSettingList")
	}
	for _, userSetting := range userSettingList {
		if userSetting.GetTelegramUserId() == strconv.FormatInt(message.From.ID, 10) {
			creatorID = userSetting.UserId
		}
	}

	if creatorID == 0 {
		_, err := bot.EditMessage(ctx, message.Chat.ID, reply.MessageID, fmt.Sprintf("Please set your telegram userid %d in UserSetting of memos", message.From.ID), nil)
		return err
	}

	create := &store.Memo{
		ResourceName: shortuuid.New(),
		CreatorID:    creatorID,
		Visibility:   store.Private,
	}

	if message.Text != nil {
		create.Content = convertToMarkdown(*message.Text, message.Entities)
	}

	if message.Caption != nil {
		create.Content = convertToMarkdown(*message.Caption, message.CaptionEntities)
	}

	if message.ForwardFromChat != nil {
		create.Content += fmt.Sprintf("\n\n[Message link](%s)", message.GetMessageLink())
	}

	memoMessage, err := t.store.CreateMemo(ctx, create)
	if err != nil {
		_, err := bot.EditMessage(ctx, message.Chat.ID, reply.MessageID, fmt.Sprintf("Failed to CreateMemo: %s", err), nil)
		return err
	}

	// create resources
	for _, attachment := range attachments {
		// Fill the common field of create
		create := store.Resource{
			ResourceName: shortuuid.New(),
			CreatorID:    creatorID,
			Filename:     filepath.Base(attachment.FileName),
			Type:         attachment.GetMimeType(),
			Size:         attachment.FileSize,
			MemoID:       &memoMessage.ID,
		}

		err := apiv1.SaveResourceBlob(ctx, t.store, &create, bytes.NewReader(attachment.Data))
		if err != nil {
			_, err := bot.EditMessage(ctx, message.Chat.ID, reply.MessageID, fmt.Sprintf("Failed to SaveResourceBlob: %s", err), nil)
			return err
		}

		_, err = t.store.CreateResource(ctx, &create)
		if err != nil {
			_, err := bot.EditMessage(ctx, message.Chat.ID, reply.MessageID, fmt.Sprintf("Failed to CreateResource: %s", err), nil)
			return err
		}
	}

	keyboard := generateKeyboardForMemoID(memoMessage.ID)
	_, err = bot.EditMessage(ctx, message.Chat.ID, reply.MessageID, fmt.Sprintf("Saved as %s Memo %d", memoMessage.Visibility, memoMessage.ID), keyboard)
	return err
}

func (t *TelegramHandler) CallbackQueryHandle(ctx context.Context, bot *telegram.Bot, callbackQuery telegram.CallbackQuery) error {
	var memoID int32
	var visibility store.Visibility
	n, err := fmt.Sscanf(callbackQuery.Data, "%s %d", &visibility, &memoID)
	if err != nil || n != 2 {
		return bot.AnswerCallbackQuery(ctx, callbackQuery.ID, fmt.Sprintf("Failed to parse callbackQuery.Data %s", callbackQuery.Data))
	}

	update := store.UpdateMemo{
		ID:         memoID,
		Visibility: &visibility,
	}
	err = t.store.UpdateMemo(ctx, &update)
	if err != nil {
		return bot.AnswerCallbackQuery(ctx, callbackQuery.ID, fmt.Sprintf("Failed to call UpdateMemo %s", err))
	}

	keyboard := generateKeyboardForMemoID(memoID)
	_, err = bot.EditMessage(ctx, callbackQuery.Message.Chat.ID, callbackQuery.Message.MessageID, fmt.Sprintf("Saved as %s Memo %d", visibility, memoID), keyboard)
	if err != nil {
		return bot.AnswerCallbackQuery(ctx, callbackQuery.ID, fmt.Sprintf("Failed to EditMessage %s", err))
	}

	return bot.AnswerCallbackQuery(ctx, callbackQuery.ID, fmt.Sprintf("Success changing Memo %d to %s", memoID, visibility))
}

func generateKeyboardForMemoID(id int32) [][]telegram.InlineKeyboardButton {
	allVisibility := []store.Visibility{
		store.Public,
		store.Protected,
		store.Private,
	}

	buttons := make([]telegram.InlineKeyboardButton, 0, len(allVisibility))
	for _, v := range allVisibility {
		button := telegram.InlineKeyboardButton{
			Text:         v.String(),
			CallbackData: fmt.Sprintf("%s %d", v, id),
		}
		buttons = append(buttons, button)
	}

	return [][]telegram.InlineKeyboardButton{buttons}
}

func convertToMarkdown(text string, messageEntities []telegram.MessageEntity) string {
	insertions := make(map[int]string)

	for _, e := range messageEntities {
		var before, after string

		// this is supported by the current markdown
		switch e.Type {
		case telegram.Bold:
			before = "**"
			after = "**"
		case telegram.Italic:
			before = "*"
			after = "*"
		case telegram.Strikethrough:
			before = "~~"
			after = "~~"
		case telegram.Code:
			before = "`"
			after = "`"
		case telegram.Pre:
			before = "```" + e.Language
			after = "```"
		case telegram.TextLink:
			before = "["
			after = fmt.Sprintf(`](%s)`, e.URL)
		}

		if before != "" {
			insertions[e.Offset] += before
			insertions[e.Offset+e.Length] = after + insertions[e.Offset+e.Length]
		}
	}

	input := []rune(text)
	var output []rune
	utf16pos := 0

	for i := 0; i < len(input); i++ {
		output = append(output, []rune(insertions[utf16pos])...)
		output = append(output, input[i])
		utf16pos += len(utf16.Encode([]rune{input[i]}))
	}
	output = append(output, []rune(insertions[utf16pos])...)

	return string(output)
}
