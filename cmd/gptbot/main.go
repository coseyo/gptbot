package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/RussellLuo/kun/pkg/httpcodec"
	"github.com/coseyo/gptbot"
	"github.com/coseyo/gptbot/milvus"
)

var promptTmpl = `
根据所提供的上下文尽可能如实回答问题，如果答案没有包含在上下文的文本中，就说“我不知道”

上下文:
{{range .Sections -}}
* {{.}}
{{- end}}

问: {{.Question}}
答:
`

var multiTurnPromptTmpl = `
你是一个与用户沟通的客服人员，基于一个问答系统可以回答查询。你的职责包括:
1. 对于问候和客套话，直接回应用户;
2. 其他问题，如不能理解，请直接询问用户; 否则，请确保以 "{{$.Prefix}}" 开头查询系统.

例子 1:
客户: 什么是 GPT-3?
客服: {{$.Prefix}} 什么是 GPT-3?

例子 2:
客户: 它使用了多少个参数?
客服: 对不起，我不太明白你的意思.

例子 3:
客户: 什么是 GPT-3?
客服: GPT-3 是一个 AI 模型.
客户: 它使用了多少个参数?
客服: {{$.Prefix}} GPT-3使用多少个参数?

对话:
{{- range $.Turns}}
客户: {{.Question}}
客服: {{.Answer}}
{{- end}}
客户: {{$.Question}}
客服:
`

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	encoder := gptbot.NewOpenAIEncoder(apiKey, "")
	store, err := milvus.NewMilvus(&milvus.Config{
		CollectionName: "gptbot",
	})
	if err != nil {
		log.Fatalf("err: %v", err)
	}

	feeder := gptbot.NewFeeder(&gptbot.FeederConfig{
		Encoder: encoder,
		Updater: store,
	})

	bot := gptbot.NewBot(&gptbot.BotConfig{
		APIKey:              apiKey,
		Encoder:             encoder,
		Querier:             store,
		PromptTmpl:          promptTmpl,
		MultiTurnPromptTmpl: multiTurnPromptTmpl,
		TopK:                6,
		MaxTokens:           3000,
		Debug:               true,
	})

	svc := NewGPTBot(feeder, store, bot)
	r := NewHTTPRouter(svc, httpcodec.NewDefaultCodecs(nil, httpcodec.Op(
		"UploadFile", httpcodec.NewMultipartForm(0),
	)))

	errs := make(chan error, 2)
	go func() {
		log.Printf("transport=HTTP addr=%s\n", ":8080")
		errs <- http.ListenAndServe(":8080", r)
	}()
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	log.Printf("terminated, err:%v", <-errs)
}
