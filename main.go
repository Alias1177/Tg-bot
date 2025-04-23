package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"os"

	"path/filepath"
)

var userState = make(map[int64]string)
var userData = make(map[int64]map[string]string)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	// Получение значения из .env
	token := os.Getenv("TOKEN")
	if token == "" {
		log.Fatal("TOKEN not found in .env")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	db, err := sqlx.Connect("postgres", "user=postgres password=password dbname=mydb host=localhost port=6345 sslmode=disable")
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

			if len(data) > 8 && data[:8] == "country_" {
				countryCode := data[8:]
				userData[chatID]["country"] = countryCode
				_, err := db.Exec("INSERT INTO users (email, country) VALUES ($1, $2)", userData[chatID]["email"], countryCode)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Failed to save your data"))
					log.Println(err)
				} else {
					userState[chatID] = "awaiting_payment"
					msg := tgbotapi.NewMessage(chatID, "Choose your payment method:")
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonURL("💳 Stripe/Crypto", "https://animego.org"),
							tgbotapi.NewInlineKeyboardButtonURL("🌍 СНГ (Multicard)", "https://animego.org"),
							tgbotapi.NewInlineKeyboardButtonData("🔧 Developer Test", "pay_check"),
						),
					)
					bot.Send(msg)
				}
			} else {
				switch data {
				case "pay_check":
					userState[chatID] = "awaiting_language"
					msg := tgbotapi.NewMessage(chatID, "Choose book language:")
					msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("🇩🇪 Deutsch", "DE"),
							tgbotapi.NewInlineKeyboardButtonData("🇬🇧 English", "EN"),
						),
						tgbotapi.NewInlineKeyboardRow(
							tgbotapi.NewInlineKeyboardButtonData("🇪🇸 Español", "ES"),
							tgbotapi.NewInlineKeyboardButtonData("🇷🇺 Русский", "RU"),
							tgbotapi.NewInlineKeyboardButtonData("🇹🇷 Türkçe", "TR"),
						),
					)
					bot.Send(msg)
				case "DE", "EN", "ES", "RU", "TR":
					userState[chatID] = "awaiting_language"
					msg := tgbotapi.NewMessage(chatID, "⏳ Please wait, sending the book...")
					msg.ProtectContent = true
					bot.Send(msg)

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
