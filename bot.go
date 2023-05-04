package gptbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/rakyll/openai-go"
	"github.com/rakyll/openai-go/chat"
	"github.com/rakyll/openai-go/completion"
)

type ModelType string

const (
	// GPT-4
	GPT4 ModelType = "gpt-4"
	// GPT-3.5
	GPT3Dot5Turbo  ModelType = "gpt-3.5-turbo"
	TextDavinci003 ModelType = "text-davinci-003"
	TextDavinci002 ModelType = "text-davinci-002"
	// GPT-3
	TextAda001     ModelType = "text-ada-001"
	TextCurie001   ModelType = "text-curie-001"
	TextBabbage001 ModelType = "text-babbage-001"
)

type Encoder interface {
	Encode(cxt context.Context, text string) (Embedding, error)
	EncodeBatch(cxt context.Context, texts []string) ([]Embedding, error)
}

type Querier interface {
	Query(ctx context.Context, embedding Embedding, topK int) ([]*Similarity, error)
}

// Turn represents a round of dialogue.
type Turn struct {
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"`
}

type BotConfig struct {
	// APIKey is the OpenAI's APIKey.
	// This field is required.
	APIKey string

	// Encoder is an Embedding Encoder, which will encode the user's question into a vector for similarity search.
	// This field is required.
	Encoder Encoder

	// Querier is a search engine, which is capable of doing the similarity search.
	// This field is required.
	Querier Querier

	// Model is the ID of OpenAI's model to use for chat.
	// Defaults to "gpt-3.5-turbo".
	Model ModelType

	// TopK specifies how many candidate similarities will be selected to construct the prompt.
	// Defaults to 3.
	TopK int

	// MaxTokens is the maximum number of tokens to generate in the chat.
	// Defaults to 256.
	MaxTokens int

	// PromptTmpl specifies a custom prompt template for single-turn mode.
	// Defaults to DefaultPromptTmpl.
	PromptTmpl string

	// MultiTurnPromptTmpl specifies a custom prompt template for multi-turn mode.
	// Defaults to DefaultMultiTurnPromptTmpl.
	//
	// Prompt-based question answering bot essentially operates in single-turn mode,
	// since the quality of each answer largely depends on the associated prompt context
	// (i.e. the most similar document chunks), which all depends on the corresponding
	// question rather than the conversation history.
	//
	// As a workaround, we try to achieve the effect of multi-turn mode by adding an
	// extra frontend agent, who can respond directly to the user for casual greetings,
	// and can refine incomplete questions according to the conversation history
	// before consulting the backend system (i.e. the single-turn Question Answering Bot).
	MultiTurnPromptTmpl string

	// Debug set true to show the http request and response message
	Debug bool

	// DictPaths is the jieba dictionary paths.
	DictPaths []string
}

func (cfg *BotConfig) init() {
	if cfg.Model == "" {
		cfg.Model = GPT3Dot5Turbo
	}
	if cfg.TopK == 0 {
		cfg.TopK = 3
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 256
	}
	if cfg.PromptTmpl == "" {
		cfg.PromptTmpl = DefaultPromptTmpl
	}
	if cfg.MultiTurnPromptTmpl == "" {
		cfg.MultiTurnPromptTmpl = DefaultMultiTurnPromptTmpl
	}
}

type Bot struct {
	cfg         *BotConfig
	chatClient  *chat.Client
	compClient  *completion.Client
	wordSegment *WordSegment
}

func NewBot(cfg *BotConfig) *Bot {
	cfg.init()
	s := openai.NewSession(cfg.APIKey)
	bot := &Bot{cfg: cfg}

	// https://platform.openai.com/docs/models/model-endpoint-compatibility
	switch cfg.Model {
	case GPT4, GPT3Dot5Turbo:
		bot.chatClient = chat.NewClient(s, string(cfg.Model))
	case TextDavinci003, TextDavinci002, TextAda001, TextBabbage001, TextCurie001:
		bot.compClient = completion.NewClient(s, string(cfg.Model))
	default:
		panic("unsupported gpt model!")
	}

	bot.wordSegment = NewWordSegment(cfg.DictPaths...)

	return bot
}

// Chat answers the given question in single-turn mode by default. If non-empty history
// is specified, multi-turn mode will be enabled. See BotConfig.MultiTurnPromptTmpl for more details.
func (b *Bot) Chat(ctx context.Context, question string, history ...*Turn) (string, error) {
	if len(history) > 0 {
		return b.multiTurnChat(ctx, question, history...)
	}
	return b.singleTurnChat(ctx, question)
}

func (b *Bot) multiTurnChat(ctx context.Context, question string, history ...*Turn) (string, error) {
	prefix := "QUERY:"

	t := PromptTemplate(b.cfg.MultiTurnPromptTmpl)
	prompt, err := t.Render(struct {
		Turns    []*Turn
		Question string
		Prefix   string
	}{
		Turns:    history,
		Question: question,
		Prefix:   prefix,
	})
	if err != nil {
		return "", err
	}

	refinedQuestionOrReply, err := b.chat(ctx, prompt)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(refinedQuestionOrReply, prefix) {
		return b.singleTurnChat(ctx, refinedQuestionOrReply[len(prefix):])
	} else {
		return refinedQuestionOrReply, nil
	}
}

func (b *Bot) singleTurnChat(ctx context.Context, question string) (string, error) {
	// chinese word segment
	question = b.wordSegment.Segment(question)
	prompt, err := b.cfg.constructPrompt(ctx, question)
	if err != nil {
		return "", err
	}
	return b.chat(ctx, prompt)
}

func (b *Bot) chat(ctx context.Context, prompt string) (string, error) {
	if b.chatClient != nil {
		return b.doChatCompletion(ctx, prompt)
	}
	return b.doCompletion(ctx, prompt)
}

// powered by /v1/chat/completions completion api, supported model like `gpt-3.5-turbo`
func (b *Bot) doChatCompletion(ctx context.Context, prompt string) (string, error) {
	resp, err := b.chatClient.CreateCompletion(ctx, &chat.CreateCompletionParams{
		MaxTokens: b.cfg.MaxTokens,
		Messages: []*chat.Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	})
	defer b.log(prompt, resp)
	if err != nil {
		return "", err
	}

	answer := resp.Choices[0].Message.Content
	return answer, nil
}

// powered by /v1/completions completion api, supported model like `text-davinci-003`
func (b *Bot) doCompletion(ctx context.Context, prompt string) (string, error) {
	resp, err := b.compClient.Create(ctx, &completion.CreateParams{
		MaxTokens: b.cfg.MaxTokens,
		Prompt:    []string{prompt},
	})
	defer b.log(prompt, resp)
	if err != nil {
		return "", err
	}

	answer := resp.Choices[0].Text
	return answer, nil
}

func (b *BotConfig) constructPrompt(ctx context.Context, question string) (string, error) {
	emb, err := b.Encoder.Encode(ctx, question)
	if err != nil {
		return "", err
	}

	similarities, err := b.Querier.Query(ctx, emb, b.TopK)
	if err != nil {
		return "", err
	}

	var texts []string
	for _, s := range similarities {
		texts = append(texts, s.Text)
	}

	p := PromptTemplate(b.PromptTmpl)
	return p.Render(PromptData{
		Question: question,
		Sections: texts,
	})
}

func (b *Bot) log(req, resp any) {
	if b.cfg.Debug {
		b, _ := json.Marshal(resp)
		fmt.Printf("req:\n%v\n\nresp:\n%v\n\n", req, string(b))
	}
}

type PromptData struct {
	Question string
	Sections []string
}

type PromptTemplate string

func (p PromptTemplate) Render(data any) (string, error) {
	tmpl, err := template.New("").Parse(string(p))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

const (
	DefaultPromptTmpl = `
Answer the question as truthfully as possible using the provided context, and if the answer is not contained within the text below, say "I don't know."

Context:

{{range .Sections -}}
* {{.}}
{{- end}}

Q: {{.Question}}
A:
`

	DefaultMultiTurnPromptTmpl = `You are an Agent who communicates with the User, with a System available for answering queries. Your responsibilities include:
1. For greetings and pleasantries, respond directly to the User;
2. For other questions, if you cannot understand them, ask the User directly; otherwise, be sure to begin with "{{$.Prefix}}" when querying the System.

Example 1:
User: What is GPT-3?
Agent: {{$.Prefix}} What is GPT-3?

Example 2:
User: How many parameters does it use?
Agent: Sorry, I don't quite understand what you mean.

Example 3:
User: What is GPT-3?
Agent: GPT-3 is an AI model.
User: How many parameters does it use?
Agent: {{$.Prefix}} How many parameters does GPT-3 use?

Conversation:
{{- range $.Turns}}
User: {{.Question}}
Agent: {{.Answer}}
{{- end}}
User: {{$.Question}}
Agent:
`
)
