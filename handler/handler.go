package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/panjf2000/ants/v2"
	"github.com/zijiren233/go-colorlog"
	"github.com/zijiren233/stable-diffusion-webui-bot/cache"
	"github.com/zijiren233/stable-diffusion-webui-bot/utils"
	tgbotapi "github.com/zijiren233/tg-bot-api/v6"
)

type Handler struct {
	bot            *tgbotapi.BotAPI
	ch             tgbotapi.UpdatesChannel
	tgToken        string
	ownerID        int64
	webhookHost    string
	webhookHandler func(w http.ResponseWriter, r *http.Request)
	webhookEnabled bool
	cache          cache.Cache
}

type ConfigFunc func(h *Handler)

// https only
func WithWebhook(webhookHost string) ConfigFunc {
	return func(h *Handler) { h.webhookHost = webhookHost }
}

func WithOwnerID(id int64) ConfigFunc {
	return func(h *Handler) { h.ownerID = id }
}

func WithCache(cache cache.Cache) ConfigFunc {
	return func(h *Handler) { h.cache = cache }
}

func New(tgToken string, configs ...ConfigFunc) (*Handler, error) {
	h := &Handler{tgToken: tgToken}
	for _, cf := range configs {
		cf(h)
	}

	bot, err := tgbotapi.NewBotAPI(tgToken)
	if err != nil {
		return nil, err
	}
	h.bot = bot
	bot.Buffer = 1000
	bot.Debug = false

	if h.cache == nil {
		h.cache, err = cache.NewCache(cache.WithSavePath(path.Join(os.TempDir(), "local-cache", bot.Token)))
		if err != nil {
			return nil, err
		}
	}

	if h.webhookHost != "" {
		wh, _ := tgbotapi.NewWebhook(fmt.Sprintf("https://%s/api/bot/%s", h.webhookHost, h.bot.Token))
		wh.MaxConnections = 100
		wh.DropPendingUpdates = true
		wh.AllowedUpdates = []string{"message", "callback_query"}
		_, err = bot.Request(wh)
		if err != nil {
			colorlog.Fatalf("Request wh err: %v", err)
			panic(err)
		}
		h.ch, h.webhookHandler = bot.NewWebhookHandler()
		h.webhookEnabled = true
	} else {
		bot.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: true})
		h.ch = bot.GetUpdatesChan(tgbotapi.NewUpdate(0))
	}

	return h, nil
}

func (h *Handler) Bot() *tgbotapi.BotAPI {
	return h.bot
}

func (h *Handler) WebhookEnabled() bool {
	return h.webhookEnabled
}

func (h *Handler) WebhookUriPath() string {
	return fmt.Sprintf("/api/bot/%s", h.bot.Token)
}

// only Enable Webhook
func (h *Handler) WebhookHandler() func(w http.ResponseWriter, r *http.Request) {
	return h.webhookHandler
}

func (h *Handler) Cache() cache.Cache {
	return h.cache
}

func (h *Handler) Run(ctx context.Context) {
	limiter := utils.NewRateLimiter(3, 1)
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-h.ch:
			if update.Message != nil && update.Message.Chat.ID > 0 {
				ants.Submit(func() {
					if !limiter.GetLimiter(update.Message.From.ID).Allow() {
						return
					}
					if update.Message.IsCommand() {
						colorlog.Infof("Get the message cmd [%s] : %s", update.Message.From.String(), update.Message.Command())
						h.HandleCmd(*update.Message)
					} else if msgChan, ok := h.bot.FindMsgCbk(update.Message.Chat.ID, update.Message.From.ID); ok {
						select {
						case msgChan.MsgChan() <- update.Message:
							colorlog.Infof("Get the message cbk [%s] : %s", update.Message.From.String(), update.Message.Text)
						default:
						}
					} else {
						colorlog.Infof("Get the message [%s] : %s%s", update.Message.From.String(), update.Message.Text, update.Message.Caption)
						h.HandleMsg(update.Message)
					}
				})
			} else if update.CallbackQuery != nil && update.CallbackQuery.Message.Chat.ID > 0 {
				ants.Submit(func() {
					if !limiter.GetLimiter(update.CallbackQuery.From.ID).Allow() {
						return
					}
					colorlog.Infof("Get the Callback [%s] : %s", update.CallbackQuery.From.String(), update.CallbackQuery.Data)
					h.HandleCallback(update.CallbackQuery)
				})
			}
		}
	}
}

func (h *Handler) SetCommand() {
	var cmds tgbotapi.SetMyCommandsConfig

	h.bot.Send(tgbotapi.NewDeleteMyCommands())

	bcs := tgbotapi.NewBotCommandScopeAllPrivateChats()
	owner := tgbotapi.NewBotCommandScopeChat(h.ownerID)

	{
		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "Get invitation link"}, tgbotapi.BotCommand{Command: "web", Description: "Use Web Site version"}, tgbotapi.BotCommand{Command: "api", Description: "Get API documentation"}, tgbotapi.BotCommand{Command: "setdefault", Description: "Set default parameters"}, tgbotapi.BotCommand{Command: "subscribe", Description: "View subscription information"}, tgbotapi.BotCommand{Command: "share", Description: "Share images to image waterfall"}, tgbotapi.BotCommand{Command: "guesstag", Description: "Guess image tags"}, tgbotapi.BotCommand{Command: "superresolution", Description: "Image super-resolution"}, tgbotapi.BotCommand{Command: "help", Description: "Help"}, tgbotapi.BotCommand{Command: "history", Description: "View history of generated images"}, tgbotapi.BotCommand{Command: "language", Description: "Set language"}, tgbotapi.BotCommand{Command: "img2tag", Description: "Image to Tag conversion"})
		cmds.LanguageCode = ""
		cmds.Scope = &bcs
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "获取邀请链接"}, tgbotapi.BotCommand{Command: "web", Description: "使用 Web Site 版本"}, tgbotapi.BotCommand{Command: "api", Description: "获取 API 接口文档"}, tgbotapi.BotCommand{Command: "setdefault", Description: "设置默认参数"}, tgbotapi.BotCommand{Command: "subscribe", Description: "查看订阅信息"}, tgbotapi.BotCommand{Command: "share", Description: "公开图片到图片瀑布流"}, tgbotapi.BotCommand{Command: "guesstag", Description: "猜测图片Tag"}, tgbotapi.BotCommand{Command: "superresolution", Description: "图片超分辨率"}, tgbotapi.BotCommand{Command: "help", Description: "帮助"}, tgbotapi.BotCommand{Command: "history", Description: "查看历史生成图片"}, tgbotapi.BotCommand{Command: "language", Description: "设置语言"}, tgbotapi.BotCommand{Command: "img2tag", Description: "图片转Tag"})
		cmds.LanguageCode = "zh"
		cmds.Scope = &bcs
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "招待リンクを取得する"}, tgbotapi.BotCommand{Command: "web", Description: "Web Site バージョンを使用する"}, tgbotapi.BotCommand{Command: "api", Description: "API インターフェースドキュメントを取得する"}, tgbotapi.BotCommand{Command: "setdefault", Description: "デフォルトパラメータを設定する"}, tgbotapi.BotCommand{Command: "subscribe", Description: "サブスクリプション情報を表示する"}, tgbotapi.BotCommand{Command: "share", Description: "画像を画像ウォーターフォールに公開する"}, tgbotapi.BotCommand{Command: "guesstag", Description: "画像タグを推測する"}, tgbotapi.BotCommand{Command: "superresolution", Description: "画像の超解像度"}, tgbotapi.BotCommand{Command: "help", Description: "ヘルプ"}, tgbotapi.BotCommand{Command: "history", Description: "過去の生成画像を見る"}, tgbotapi.BotCommand{Command: "language", Description: "言語を設定する"}, tgbotapi.BotCommand{Command: "img2tag", Description: "画像をタグに変換する"})
		cmds.LanguageCode = "ja"
		cmds.Scope = &bcs
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "초대 링크 얻기"}, tgbotapi.BotCommand{Command: "web", Description: "웹 사이트 버전 사용"}, tgbotapi.BotCommand{Command: "api", Description: "API 인터페이스 문서 가져오기"}, tgbotapi.BotCommand{Command: "setdefault", Description: "기본 매개변수 설정"}, tgbotapi.BotCommand{Command: "subscribe", Description: "구독 정보 확인"}, tgbotapi.BotCommand{Command: "share", Description: "이미지를 이미지 물방울 흐름에 공개"}, tgbotapi.BotCommand{Command: "guesstag", Description: "이미지 태그 추측"}, tgbotapi.BotCommand{Command: "superresolution", Description: "이미지 초고해상도"}, tgbotapi.BotCommand{Command: "help", Description: "도움말"}, tgbotapi.BotCommand{Command: "history", Description: "과거 생성된 이미지 확인"}, tgbotapi.BotCommand{Command: "language", Description: "언어 설정"}, tgbotapi.BotCommand{Command: "img2tag", Description: "이미지를 태그로 변환"})
		cmds.LanguageCode = "ko"
		cmds.Scope = &bcs
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "Obter link de convite"}, tgbotapi.BotCommand{Command: "web", Description: "Usar a versão do Web Site"}, tgbotapi.BotCommand{Command: "api", Description: "Obter documentação da API"}, tgbotapi.BotCommand{Command: "setdefault", Description: "Definir parâmetros padrão"}, tgbotapi.BotCommand{Command: "subscribe", Description: "Ver informações de inscrição"}, tgbotapi.BotCommand{Command: "share", Description: "Compartilhar imagem publicamente no fluxo de imagens"}, tgbotapi.BotCommand{Command: "guesstag", Description: "Adivinhar Tag da imagem"}, tgbotapi.BotCommand{Command: "superresolution", Description: "Super resolução de imagem"}, tgbotapi.BotCommand{Command: "help", Description: "Ajuda"}, tgbotapi.BotCommand{Command: "history", Description: "Ver histórico de imagens geradas"}, tgbotapi.BotCommand{Command: "language", Description: "Definir idioma"}, tgbotapi.BotCommand{Command: "img2tag", Description: "Converter imagem em Tag"})
		cmds.LanguageCode = "pt"
		cmds.Scope = &bcs
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "invite", Description: "получить ссылку-приглашение"}, tgbotapi.BotCommand{Command: "web", Description: "использовать версию Web Site"}, tgbotapi.BotCommand{Command: "api", Description: "получить документацию API"}, tgbotapi.BotCommand{Command: "setdefault", Description: "установить параметры по умолчанию"}, tgbotapi.BotCommand{Command: "subscribe", Description: "просмотреть информацию о подписке"}, tgbotapi.BotCommand{Command: "share", Description: "опубликовать изображение в потоке изображений"}, tgbotapi.BotCommand{Command: "guesstag", Description: "угадать тег изображения"}, tgbotapi.BotCommand{Command: "superresolution", Description: "суперразрешение изображения"}, tgbotapi.BotCommand{Command: "help", Description: "помощь"}, tgbotapi.BotCommand{Command: "history", Description: "просмотр истории созданных изображений"}, tgbotapi.BotCommand{Command: "language", Description: "установить язык"}, tgbotapi.BotCommand{Command: "img2tag", Description: "преобразование изображения в тег"})
		cmds.LanguageCode = "ru"
		cmds.Scope = &bcs
		h.bot.Send(cmds)
	}

	{
		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "Get backend pool info"}, tgbotapi.BotCommand{Command: "info", Description: "Get user info with user id: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "Get token with days: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "Get invitation link"}, tgbotapi.BotCommand{Command: "web", Description: "Use Web Site version"}, tgbotapi.BotCommand{Command: "api", Description: "Get API documentation"}, tgbotapi.BotCommand{Command: "setdefault", Description: "Set default parameters"}, tgbotapi.BotCommand{Command: "subscribe", Description: "View subscription information"}, tgbotapi.BotCommand{Command: "share", Description: "Share images to image waterfall"}, tgbotapi.BotCommand{Command: "guesstag", Description: "Guess image tags"}, tgbotapi.BotCommand{Command: "superresolution", Description: "Image super-resolution"}, tgbotapi.BotCommand{Command: "help", Description: "Help"}, tgbotapi.BotCommand{Command: "history", Description: "View history of generated images"}, tgbotapi.BotCommand{Command: "language", Description: "Set language"}, tgbotapi.BotCommand{Command: "img2tag", Description: "Image to Tag conversion"})
		cmds.LanguageCode = ""
		cmds.Scope = &owner
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "获取后端池信息"}, tgbotapi.BotCommand{Command: "info", Description: "通过用户id获取用户信息: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "获取带天数的令牌: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "获取邀请链接"}, tgbotapi.BotCommand{Command: "web", Description: "使用 Web Site 版本"}, tgbotapi.BotCommand{Command: "api", Description: "获取 API 接口文档"}, tgbotapi.BotCommand{Command: "setdefault", Description: "设置默认参数"}, tgbotapi.BotCommand{Command: "subscribe", Description: "查看订阅信息"}, tgbotapi.BotCommand{Command: "share", Description: "公开图片到图片瀑布流"}, tgbotapi.BotCommand{Command: "guesstag", Description: "猜测图片Tag"}, tgbotapi.BotCommand{Command: "superresolution", Description: "图片超分辨率"}, tgbotapi.BotCommand{Command: "help", Description: "帮助"}, tgbotapi.BotCommand{Command: "history", Description: "查看历史生成图片"}, tgbotapi.BotCommand{Command: "language", Description: "设置语言"}, tgbotapi.BotCommand{Command: "img2tag", Description: "图片转Tag"})
		cmds.LanguageCode = "zh"
		cmds.Scope = &owner
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "バックエンド プール情報を取得する"}, tgbotapi.BotCommand{Command: "info", Description: "ユーザーIDを使用してユーザー情報を取得する: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "日数のトークンを取得する: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "招待リンクを取得する"}, tgbotapi.BotCommand{Command: "web", Description: "Web Site バージョンを使用する"}, tgbotapi.BotCommand{Command: "api", Description: "API インターフェースドキュメントを取得する"}, tgbotapi.BotCommand{Command: "setdefault", Description: "デフォルトパラメータを設定する"}, tgbotapi.BotCommand{Command: "subscribe", Description: "サブスクリプション情報を表示する"}, tgbotapi.BotCommand{Command: "share", Description: "画像を画像ウォーターフォールに公開する"}, tgbotapi.BotCommand{Command: "guesstag", Description: "画像タグを推測する"}, tgbotapi.BotCommand{Command: "superresolution", Description: "画像の超解像度"}, tgbotapi.BotCommand{Command: "help", Description: "ヘルプ"}, tgbotapi.BotCommand{Command: "history", Description: "過去の生成画像を見る"}, tgbotapi.BotCommand{Command: "language", Description: "言語を設定する"}, tgbotapi.BotCommand{Command: "img2tag", Description: "画像をタグに変換する"})
		cmds.LanguageCode = "ja"
		cmds.Scope = &owner
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "백엔드 풀 정보 가져오기"}, tgbotapi.BotCommand{Command: "info", Description: "사용자 ID로 사용자 정보 가져오기: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "일 단위로 토큰 받기: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "초대 링크 얻기"}, tgbotapi.BotCommand{Command: "web", Description: "웹 사이트 버전 사용"}, tgbotapi.BotCommand{Command: "api", Description: "API 인터페이스 문서 가져오기"}, tgbotapi.BotCommand{Command: "setdefault", Description: "기본 매개변수 설정"}, tgbotapi.BotCommand{Command: "subscribe", Description: "구독 정보 확인"}, tgbotapi.BotCommand{Command: "share", Description: "이미지를 이미지 물방울 흐름에 공개"}, tgbotapi.BotCommand{Command: "guesstag", Description: "이미지 태그 추측"}, tgbotapi.BotCommand{Command: "superresolution", Description: "이미지 초고해상도"}, tgbotapi.BotCommand{Command: "help", Description: "도움말"}, tgbotapi.BotCommand{Command: "history", Description: "과거 생성된 이미지 확인"}, tgbotapi.BotCommand{Command: "language", Description: "언어 설정"}, tgbotapi.BotCommand{Command: "img2tag", Description: "이미지를 태그로 변환"})
		cmds.LanguageCode = "ko"
		cmds.Scope = &owner
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "Obter informações do pool de back-end"}, tgbotapi.BotCommand{Command: "info", Description: "Obter informações do usuário com ID do usuário: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "Obter token com dias: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "Obter link de convite"}, tgbotapi.BotCommand{Command: "web", Description: "Usar a versão do Web Site"}, tgbotapi.BotCommand{Command: "api", Description: "Obter documentação da API"}, tgbotapi.BotCommand{Command: "setdefault", Description: "Definir parâmetros padrão"}, tgbotapi.BotCommand{Command: "subscribe", Description: "Ver informações de inscrição"}, tgbotapi.BotCommand{Command: "share", Description: "Compartilhar imagem publicamente no fluxo de imagens"}, tgbotapi.BotCommand{Command: "guesstag", Description: "Adivinhar Tag da imagem"}, tgbotapi.BotCommand{Command: "superresolution", Description: "Super resolução de imagem"}, tgbotapi.BotCommand{Command: "help", Description: "Ajuda"}, tgbotapi.BotCommand{Command: "history", Description: "Ver histórico de imagens geradas"}, tgbotapi.BotCommand{Command: "language", Description: "Definir idioma"}, tgbotapi.BotCommand{Command: "img2tag", Description: "Converter imagem em Tag"})
		cmds.LanguageCode = "pt"
		cmds.Scope = &owner
		h.bot.Send(cmds)

		cmds = tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{Command: "pool", Description: "Получить информацию о внутреннем пуле"}, tgbotapi.BotCommand{Command: "info", Description: "Получить информацию о пользователе с идентификатором пользователя: /info <userid>"}, tgbotapi.BotCommand{Command: "gettoken", Description: "Получите токен за дни: /gettoken 30"}, tgbotapi.BotCommand{Command: "invite", Description: "получить ссылку-приглашение"}, tgbotapi.BotCommand{Command: "web", Description: "использовать версию Web Site"}, tgbotapi.BotCommand{Command: "api", Description: "получить документацию API"}, tgbotapi.BotCommand{Command: "setdefault", Description: "установить параметры по умолчанию"}, tgbotapi.BotCommand{Command: "subscribe", Description: "просмотреть информацию о подписке"}, tgbotapi.BotCommand{Command: "share", Description: "опубликовать изображение в потоке изображений"}, tgbotapi.BotCommand{Command: "guesstag", Description: "угадать тег изображения"}, tgbotapi.BotCommand{Command: "superresolution", Description: "суперразрешение изображения"}, tgbotapi.BotCommand{Command: "help", Description: "помощь"}, tgbotapi.BotCommand{Command: "history", Description: "просмотр истории созданных изображений"}, tgbotapi.BotCommand{Command: "language", Description: "установить язык"}, tgbotapi.BotCommand{Command: "img2tag", Description: "преобразование изображения в тег"})
		cmds.LanguageCode = "ru"
		cmds.Scope = &owner
		h.bot.Send(cmds)
	}

}
