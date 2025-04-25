package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v82"
	checkout "github.com/stripe/stripe-go/v82/checkout/session"
)

var countryPriceMap = map[string]string{
	"ES":  "price_1RHUWn4ghrcCQfAXXF4mPkHw",
	"CA":  "price_1RHUWn4ghrcCQfAXXF4mPkHw",
	"US":  "price_1RHUWn4ghrcCQfAXXF4mPkHw",
	"TR":  "price_1RHUWn4ghrcCQfAXXF4mPkHw",
	"CIS": "price_1RHUWn4ghrcCQfAXXF4mPkHw",
	"AS":  "price_1RHUWn4ghrcCQfAXXF4mPkHw",
}

func main() {
	// Загрузка переменных окружения из .env (если есть)
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment")
	}

	// Установка ключа Stripe из переменной окружения
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		log.Fatal("STRIPE_SECRET_KEY not set")
	}

	// Маршрут для создания сессии
	http.HandleFunc("/create-checkout-session", handleCreateSession)

	// (Опционально) раздача статических файлов
	http.Handle("/", http.FileServer(http.Dir("public")))

	addr := "localhost:4242"
	log.Printf("Stripe Checkout server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleCreateSession создаёт Checkout Session по коду страны из query-параметра
func handleCreateSession(w http.ResponseWriter, r *http.Request) {
	// читаем код страны из URL: /create-checkout-session?country=CA
	country := r.URL.Query().Get("country")
	if country == "" {
		country = "ES"
	}
	priceID, ok := countryPriceMap[country]
	if !ok {
		priceID = countryPriceMap["ES"]
	}

	domain := "http://localhost:4242"
	params := &stripe.CheckoutSessionParams{
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(domain + "/success.html"),
		CancelURL:  stripe.String(domain + "/cancel.html"),
	}

	s, err := checkout.New(params)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "Could not create checkout session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": s.URL})
}
