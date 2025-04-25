module RestApiServer/Tg-bot

go 1.23.2

require github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.2-0.20221020003552-4126fa611266

require (
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/stripe/stripe-go/v82 v82.0.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect
