package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	userState = make(map[int64]string)
	userData  = make(map[int64]map[string]string)
)

func main() {
	_ = godotenv.Load()

	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("TOKEN not set in environment")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	// Соединение с базой
	db, err := sqlx.Connect("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		chatID := update.FromChat().ID
		if _, ok := userData[chatID]; !ok {
			userData[chatID] = make(map[string]string)
		}

		if update.Message != nil && update.Message.IsCommand() {
			if update.Message.Command() == "start" {
				userState[chatID] = "awaiting_email"
				bot.Send(tgbotapi.NewMessage(chatID, "Please enter your email:"))
			}

		} else if update.Message != nil {
			switch userState[chatID] {
			case "awaiting_email":
				userData[chatID]["email"] = update.Message.Text
				userState[chatID] = "awaiting_country"

				msg := tgbotapi.NewMessage(chatID, "Select your region:")
				msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🇪🇺 EU", "country_ES"),
						tgbotapi.NewInlineKeyboardButtonData("🇨🇦 Canada", "country_CA"),
						tgbotapi.NewInlineKeyboardButtonData("🇺🇸 USA", "country_US"),
					),
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🇹🇷 Türkiye", "country_TR"),
						tgbotapi.NewInlineKeyboardButtonData("🌍 СНГ", "country_CIS"),
						tgbotapi.NewInlineKeyboardButtonData("🌏 Asia", "country_AS"),
					),
				)
				bot.Send(msg)

			case "awaiting_language":
				lang := update.Message.Text
				waitMsg := tgbotapi.NewMessage(chatID, "⏳ Please wait, sending the book...")
				waitMsg.ProtectContent = true
				bot.Send(waitMsg)

				filePath := filepath.Join("pfdSender", "Trade-Plus.Online:"+lang+".pdf")
				doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
				doc.Caption = "📘 Your book in: " + lang
				doc.ProtectContent = true
				bot.Send(doc)

				delete(userState, chatID)
			}

		} else if update.CallbackQuery != nil {
			data := update.CallbackQuery.Data

			if strings.HasPrefix(data, "country_") {
				// Сохраняем email + country в БД
				countryCode := strings.TrimPrefix(data, "country_")
				userData[chatID]["country"] = countryCode

				_, err := db.Exec(
					"INSERT INTO users (email, country) VALUES ($1, $2)",
					userData[chatID]["email"], countryCode,
				)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to save your data"))
					log.Println(err)
				} else {
					userState[chatID] = "awaiting_payment"
					msg := tgbotapi.NewMessage(chatID, "Choose your payment method:")
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("💳 Stripe/Crypto", "pay_stripe"),
						),
					)
					bot.Send(msg)
				}

			} else {
				switch data {
				case "pay_stripe":
					// Запрашиваем URL с учётом страны
					country := userData[chatID]["country"]
					url, err := getCheckoutURL(country)
					if err != nil {
						bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to start payment"))
						log.Println(err)
					} else {
						bot.Send(tgbotapi.NewMessage(chatID, "💳 Please pay here:\n"+url))
						// После — предлагаем выбрать язык
						msg := tgbotapi.NewMessage(chatID, "Please select book language:")
						msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("🇩🇪 German", "DE"),
								tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "EN"),
								tgbotapi.NewInlineKeyboardButtonData("🇪🇸 Spanish", "ES"),
							),
							tgbotapi.NewInlineKeyboardRow(
								tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Russian", "RU"),
								tgbotapi.NewInlineKeyboardButtonData("🇹🇷 Turkish", "TR"),
							),
						)
						bot.Send(msg)
						userState[chatID] = "awaiting_language"
					}

				// Обработка выбора языка при отправке PDF
				case "DE", "EN", "ES", "RU", "TR":
					userState[chatID] = "awaiting_language"
					waitMsg := tgbotapi.NewMessage(chatID, "⏳ Please wait, sending the book...")
					waitMsg.ProtectContent = true
					bot.Send(waitMsg)

					filePath := filepath.Join("pfdSender", "Trade-Plus.Online:"+data+".pdf")
					doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
					doc.Caption = "📘 Your book in: " + data
					doc.ProtectContent = true
					bot.Send(doc)

					delete(userState, chatID)
				}
			}
		}
	}
}

// getCheckoutURL запрашивает Stripe-сессию у локального сервера
func getCheckoutURL(countryCode string) (string, error) {
	resp, err := http.Get("http://localhost:4242/create-checkout-session?country=" + countryCode)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.URL, nil
}
