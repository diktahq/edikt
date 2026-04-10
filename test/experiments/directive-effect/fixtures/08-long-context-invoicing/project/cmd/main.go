package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/example/checkout/internal/handler"
	"github.com/example/checkout/internal/middleware"
	"github.com/example/checkout/internal/repository"
	"github.com/example/checkout/internal/service"
)

func main() {
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	carts := repository.NewCartsRepo(db)
	orders := repository.NewOrdersRepo(db)
	checkoutSvc := service.NewCheckoutService(carts, orders)
	checkoutH := handler.NewCheckoutHandler(checkoutSvc)

	mux := http.NewServeMux()
	authed := http.NewServeMux()
	authed.HandleFunc("POST /checkout", checkoutH.Checkout)
	mux.Handle("/", middleware.Auth(authed))

	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
