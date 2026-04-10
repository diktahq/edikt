package main

import (
	"log"
	"net/http"
	"os"

	"github.com/example/orders-service/internal/audit"
	"github.com/example/orders-service/internal/db"
	"github.com/example/orders-service/internal/email"
	"github.com/example/orders-service/internal/events"
	apphttp "github.com/example/orders-service/internal/http"
	"github.com/example/orders-service/internal/payment"
	"github.com/example/orders-service/internal/queue"
	"github.com/example/orders-service/internal/repository"
	"github.com/example/orders-service/internal/service"
)

func main() {
	dbConn, err := db.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer dbConn.Close()

	q := queue.New(os.Getenv("QUEUE_URL"))
	stripe := payment.NewStripeClient(os.Getenv("STRIPE_SECRET_KEY"))
	mailer := email.NewSender(q)
	auditor := audit.NewRecorder(dbConn)
	pub := events.NewPublisher(os.Getenv("EVENTS_BROKER_URL"))

	repos := repository.New(dbConn)
	svc := service.New(repos, stripe, auditor, pub, mailer)

	router := apphttp.NewRouter(svc)

	addr := ":" + envOr("PORT", "8080")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
