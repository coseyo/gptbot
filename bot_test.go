package gptbot_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/coseyo/gptbot"
)

func TestBot_Chat(t *testing.T) {
	ctx := context.Background()

	store := gptbot.NewLocalVectorStore()
	if err := store.LoadJSON(ctx, "testdata/olympics_sections.json"); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	bot := gptbot.NewBot(&gptbot.BotConfig{
		APIKey:  apiKey,
		Encoder: gptbot.NewOpenAIEncoder(apiKey, ""),
		Querier: store,
	})

	question := "Who won the 2020 Summer Olympics men's high jump?"
	answer, err := bot.Chat(ctx, question)
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !strings.Contains(answer, "Gianmarco Tamberi") || !strings.Contains(answer, "Mutaz Essa Barshim") {
		t.Errorf("unexpected answer: %s\n", answer)
	}
}

func TestBot_ChatWithHistory(t *testing.T) {
	ctx := context.Background()
	var history []*gptbot.Turn

	store := gptbot.NewLocalVectorStore()
	if err := store.LoadJSON(ctx, "testdata/olympics_sections.json"); err != nil {
		t.Fatalf("err: %v\n", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	bot := gptbot.NewBot(&gptbot.BotConfig{
		APIKey:  apiKey,
		Encoder: gptbot.NewOpenAIEncoder(apiKey, ""),
		Querier: store,
	})

	question := "Who won the 2020 Summer Olympics men's high jump?"
	answer, err := bot.Chat(ctx, question, history...)
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}
	history = append(history, &gptbot.Turn{
		Question: question,
		Answer:   answer,
	})

	if !strings.Contains(answer, "Gianmarco Tamberi") || !strings.Contains(answer, "Mutaz Essa Barshim") {
		t.Errorf("unexpected answer: %s\n", answer)
	}

	question = "Did they agree to share the gold medal?" // In multi-turn mode, here "they" will be inferred to the names of the winners.
	answer, err = bot.Chat(ctx, question, history...)
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}

	if !strings.Contains(answer, "Yes") || !strings.Contains(answer, "agreed to share the gold medal") {
		t.Errorf("unexpected answer: %s\n", answer)
	}
}
