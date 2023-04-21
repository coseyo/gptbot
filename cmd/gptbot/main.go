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
		APIKey:  apiKey,
		Encoder: encoder,
		Querier: store,
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
