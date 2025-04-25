package main

//5HTMBMOFR7JVLWKC5VIHYM5DDEOU2V2A
import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/webhook"
)

var (
	countryPriceMap = map[string]string{
		"ES":  "price_1RHs4cBQsV5j2DYhuZ97WR7n",
		"CA":  "price_1RHs4cBQsV5j2DYhuZ97WR7n",
		"US":  "price_1RHs4cBQsV5j2DYhuZ97WR7n",
		"TR":  "price_1RHs4cBQsV5j2DYhuZ97WR7n",
		"CIS": "price_1RHs4cBQsV5j2DYhuZ97WR7n",
		"AS":  "price_1RHs4cBQsV5j2DYhuZ97WR7n",
	}
	db      *sqlx.DB
	bot     *tgbotapi.BotAPI
	botChan chan PaymentNotification
)

// Структура для хранения данных пользователя и платежа
type User struct {
	ID      int64  `db:"id"`
	Email   string `db:"email"`
	Country string `db:"country"`
	ChatID  int64  `db:"chat_id"`
}

// Структура для платежей
type Payment struct {
	ID          string `db:"id"`
	UserID      int64  `db:"user_id"`
	Status      string `db:"status"`
	SessionID   string `db:"session_id"`
	Amount      int64  `db:"amount"`
	CreatedAt   int64  `db:"created_at"`
	CompletedAt int64  `db:"completed_at"`
}

// Структура для уведомлений между сервисами
type PaymentNotification struct {
	ChatID    int64
	Email     string
	SessionID string
	Status    string
}

func main() {
	// Загрузка переменных окружения из .env
	if err := godotenv.Load(); err != nil {
		log.Println("Предупреждение: файл .env не найден, используются системные переменные")
	}

	// Инициализация канала для коммуникации между сервисами
	botChan = make(chan PaymentNotification, 100)

	// Настройка Stripe
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		log.Fatal("STRIPE_SECRET_KEY не установлен")
	}

	// Подключение к базе данных
	var err error
	db, err = sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}

	// Создание необходимых таблиц, если они не существуют
	initDB()

	// Инициализация Telegram бота
	bot, err = tgbotapi.NewBotAPI(os.Getenv("TOKEN"))
	if err != nil {
		log.Fatal("Ошибка инициализации бота:", err)
	}

	// Запуск обработчика уведомлений о платежах
	go handlePaymentNotifications()

	// HTTP сервер для Stripe Webhook и создания сессий
	http.HandleFunc("/webhook", handleStripeWebhook)
	http.HandleFunc("/create-checkout-session", handleCreateSession)
	http.Handle("/", http.FileServer(http.Dir("public")))

	// Запуск Telegram бота в отдельной горутине
	go runTelegramBot()

	// Запуск HTTP сервера
	addr := "localhost:4242"
	log.Printf("Stripe Checkout сервер запущен на %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// Инициализация базы данных
func initDB() {
	// Создание таблицы пользователей, если не существует
	db.MustExec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email TEXT NOT NULL,
			country VARCHAR NOT NULL,
			chat_id BIGINT NOT NULL
		)
	`)

	// Создание таблицы платежей, если не существует
	db.MustExec(`
		CREATE TABLE IF NOT EXISTS payments (
			id VARCHAR PRIMARY KEY,
			user_id BIGINT REFERENCES users(id),
			status VARCHAR NOT NULL,
			session_id VARCHAR NOT NULL,
			amount BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			completed_at BIGINT
		)
	`)
}

// Обработчик создания сессии Stripe Checkout
func handleCreateSession(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "Email не указан", http.StatusBadRequest)
		return
	}

	country := r.URL.Query().Get("country")
	if country == "" {
		country = "ES" // значение по умолчанию
	}

	chatID := r.URL.Query().Get("chat_id")
	if chatID == "" {
		http.Error(w, "Chat ID не указан", http.StatusBadRequest)
		return
	}

	priceID, ok := countryPriceMap[country]
	if !ok {
		priceID = countryPriceMap["ES"]
	}

	// Создание метаданных для сессии
	metadata := make(map[string]string)
	metadata["email"] = email
	metadata["country"] = country
	metadata["chat_id"] = chatID

	// В функции handleCreateSession (примерно строка 174)
	domain := "http://localhost:4242"
	params := &stripe.CheckoutSessionParams{
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:          stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:    stripe.String(domain + "/success.html"),
		CancelURL:     stripe.String(domain + "/cancel.html"),
		CustomerEmail: stripe.String(email),
		Metadata:      metadata,
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("Ошибка создания сессии: %v", err)
		http.Error(w, "Не удалось создать сессию оплаты", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": s.URL})
}

// Обработчик webhook от Stripe
func handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Ошибка чтения тела запроса: %v", err)
		http.Error(w, "Ошибка чтения запроса", http.StatusInternalServerError)
		return
	}

	// Получение подписи из заголовка
	stripeSignature := r.Header.Get("Stripe-Signature")
	endpointSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")

	// Проверка подписи
	event, err := webhook.ConstructEvent(payload, stripeSignature, endpointSecret)
	if err != nil {
		log.Printf("Ошибка подписи webhook: %v", err)
		http.Error(w, "Неверная подпись", http.StatusBadRequest)
		return
	}

	// Обработка событий Stripe
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("Ошибка парсинга сессии: %v", err)
			http.Error(w, "Ошибка парсинга данных", http.StatusInternalServerError)
			return
		}

		// Получение информации о пользователе из метаданных
		email := session.Metadata["email"]
		chatIDStr := session.Metadata["chat_id"]

		if email == "" || chatIDStr == "" {
			log.Printf("Метаданные сессии неполные: email=%s, chat_id=%s", email, chatIDStr)
			http.Error(w, "Неполные метаданные", http.StatusBadRequest)
			return
		}

		// Отправка уведомления о платеже в канал
		notification := PaymentNotification{
			Email:     email,
			SessionID: session.ID,
			Status:    "completed",
		}
		botChan <- notification

		log.Printf("Платеж успешно обработан для %s", email)
	}

	w.WriteHeader(http.StatusOK)
}

// Обработчик уведомлений о платежах для Telegram бота
func handlePaymentNotifications() {
	for notification := range botChan {
		// Получение информации о пользователе из базы данных
		var user User
		err := db.Get(&user, "SELECT * FROM users WHERE email = $1", notification.Email)
		if err != nil {
			log.Printf("Ошибка получения пользователя: %v", err)
			continue
		}

		// Обновление статуса платежа в базе данных
		_, err = db.Exec(
			"UPDATE payments SET status = $1, completed_at = extract(epoch from now()) WHERE session_id = $2",
			notification.Status, notification.SessionID,
		)
		if err != nil {
			log.Printf("Ошибка обновления платежа: %v", err)
		}

		// Отправка сообщения пользователю
		msg := tgbotapi.NewMessage(user.ChatID, "✅ Ваш платеж успешно обработан! Пожалуйста, выберите язык книги:")
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🇩🇪 Немецкий", "DE"),
				tgbotapi.NewInlineKeyboardButtonData("🇬🇧 Английский", "EN"),
				tgbotapi.NewInlineKeyboardButtonData("🇪🇸 Испанский", "ES"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "RU"),
				tgbotapi.NewInlineKeyboardButtonData("🇹🇷 Турецкий", "TR"),
			),
		)
		bot.Send(msg)
	}
}

// Функция для запуска Telegram бота
func runTelegramBot() {
	// Похожая логика как в основном main.go, но с обновлениями
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	userState := make(map[int64]string)
	userData := make(map[int64]map[string]string)

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil {
			continue
		}

		var chatID int64
		if update.Message != nil {
			chatID = update.Message.Chat.ID
		} else if update.CallbackQuery != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		}

		// Инициализация данных пользователя, если они еще не существуют
		if _, ok := userData[chatID]; !ok {
			userData[chatID] = make(map[string]string)
		}

		// Обработка команд
		if update.Message != nil && update.Message.IsCommand() {
			if update.Message.Command() == "start" {
				userState[chatID] = "awaiting_email"
				bot.Send(tgbotapi.NewMessage(chatID, "Пожалуйста, введите ваш email:"))
			}
		} else if update.Message != nil {
			// Обработка текстовых сообщений на основе состояния пользователя
			switch userState[chatID] {
			case "awaiting_email":
				email := update.Message.Text

				// Базовая валидация email
				if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
					msg := tgbotapi.NewMessage(chatID, "❌ Пожалуйста, введите корректный email в формате example@domain.com")
					bot.Send(msg)
					continue // Пропускаем дальнейшую обработку
				}

				userData[chatID]["email"] = email
				userState[chatID] = "awaiting_country"

				msg := tgbotapi.NewMessage(chatID, "Выберите ваш регион:")
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🇪🇺 ЕС", "country_ES"),
						tgbotapi.NewInlineKeyboardButtonData("🇨🇦 Канада", "country_CA"),
						tgbotapi.NewInlineKeyboardButtonData("🇺🇸 США", "country_US"),
					),
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🇹🇷 Турция", "country_TR"),
						tgbotapi.NewInlineKeyboardButtonData("🌍 СНГ", "country_CIS"),
						tgbotapi.NewInlineKeyboardButtonData("🌏 Азия", "country_AS"),
					),
				)
				bot.Send(msg)
			}
		} else if update.CallbackQuery != nil {
			data := update.CallbackQuery.Data

			if data == "DE" || data == "EN" || data == "ES" || data == "RU" || data == "TR" {
				// Отправка PDF файла на выбранном языке
				waitMsg := tgbotapi.NewMessage(chatID, "⏳ Пожалуйста, подождите, отправляю книгу...")
				waitMsg.ProtectContent = true
				bot.Send(waitMsg)

				filePath := filepath.Join("pfdSender", "Trade-Plus.Online:"+data+".pdf")
				doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
				doc.Caption = "📘 Ваша книга на: " + data
				doc.ProtectContent = true
				bot.Send(doc)

				delete(userState, chatID)
			} else if strings.HasPrefix(data, "country_") {
				// Обработка выбора страны
				countryCode := strings.TrimPrefix(data, "country_")
				userData[chatID]["country"] = countryCode

				// Сохранение данных пользователя в базе
				var userID int64
				err := db.QueryRow(
					"INSERT INTO users (email, country, chat_id) VALUES ($1, $2, $3) RETURNING id",
					userData[chatID]["email"], countryCode, chatID,
				).Scan(&userID)

				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось сохранить ваши данные"))
					log.Println(err)

				} else {
					// Создаем сессию напрямую
					priceID, ok := countryPriceMap[countryCode]
					if !ok {
						priceID = countryPriceMap["ES"]
					}

					params := &stripe.CheckoutSessionParams{
						LineItems: []*stripe.CheckoutSessionLineItemParams{
							{
								Price:    stripe.String(priceID),
								Quantity: stripe.Int64(1),
							},
						},
						Mode:          stripe.String(string(stripe.CheckoutSessionModePayment)),
						SuccessURL:    stripe.String("https://t.me/Trade_Plus_Online_Bot"),
						CancelURL:     stripe.String("https://t.me/Trade_Plus_Online_Bot"),
						CustomerEmail: stripe.String(userData[chatID]["email"]),
						Metadata: map[string]string{
							"email":   userData[chatID]["email"],
							"country": countryCode,
							"chat_id": strconv.FormatInt(chatID, 10),
						},
					}

					s, err := session.New(params)
					if err != nil {
						bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка создания сессии оплаты"))
						log.Println(err)
						return
					}

					// Отправляем прямую ссылку на Stripe Checkout
					msg := tgbotapi.NewMessage(chatID, "💳 Пожалуйста, нажмите кнопку ниже для оплаты:")
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonURL("🔒 Оплатить заказ", s.URL),
						),
					)
					bot.Send(msg)
				}
			}
		}
	}
}
