package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key, secret, callback := os.Getenv("INFRAI_API_KEY"), os.Getenv("WEBHOOK_SECRET"), os.Getenv("WEBHOOK_URL")
	if key == "" || secret == "" || callback == "" {
		log.Fatal("set INFRAI_API_KEY, WEBHOOK_SECRET and WEBHOOK_URL")
	}
	c := &client{key: key, base: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
	if err := c.registerWebhook(context.Background(), callback, secret); err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(http.ListenAndServe(addr, newService(c, secret).handler()))
}
