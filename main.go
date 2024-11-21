package main

import (
	"context"
	"fmt"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

const (
	CONFIG_API         string               = "https://ton.org/global.config.json"
	PROOF_CHECK_POLYCY ton.ProofCheckPolicy = ton.ProofCheckPolicyFast /* .ProofCheckPolicySecure*/
	MAX_CONCURRENT_TXS int                  = 10
)

var (
	client *liteclient.ConnectionPool
	api    ton.APIClientWrapped
	w      *wallet.Wallet
	db     *gorm.DB
	bot    *tgbotapi.BotAPI
	chatID string
)

func main() {
	initializeApp()

	http.HandleFunc("/sendTransactions", sendTransactionsHandler)
	http.HandleFunc("/alive", testHandler)
	go processPendingTransactions()
	http.ListenAndServe(":8888", nil)
}

func initializeApp() {
	var err error
	client := liteclient.NewConnectionPool()

	cfg, err := liteclient.GetConfigFromUrl(context.Background(), CONFIG_API)
	if err != nil {
		fmt.Println("get config err: ", err.Error())
		return
	}

	// connect to mainnet lite servers
	err = client.AddConnectionsFromConfig(context.Background(), cfg)
	if err != nil {
		fmt.Println("connection err: ", err.Error())
		return
	}

	// initialize ton api lite connection wrapper with full proof checks
	api = ton.NewAPIClient(client, PROOF_CHECK_POLYCY).WithRetry()
	api.SetTrustedBlockFromConfig(cfg)

	err = godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file", err.Error())
	}

	seedWords := os.Getenv("SEED_PHRASE")
	var words []string

	if seedWords == "" {
		log.Println("SEED_PHRASE env is empty")
		words = wallet.NewSeed()
		log.Println("Generated seed words:", strings.Join(words, " "))
	} else {
		words = strings.Split(seedWords, " ")
	}

	//w, err = wallet.FromSeed(api, words, wallet.HighloadV2R2)
	// initialize high-load wallet
	w, err = wallet.FromSeed(api, words, wallet.ConfigHighloadV3{
		MessageTTL: 60 * 5,
		MessageBuilder: func(ctx context.Context, subWalletId uint32) (id uint32, createdAt int64, err error) {
			// Due to specific of externals emulation on liteserver,
			// we need to take something less than or equals to block time, as message creation time,
			// otherwise external message will be rejected, because time will be > than emulation time
			// hope it will be fixed in the next LS versions
			createdAt = time.Now().Unix() - 30

			// example query id which will allow you to send 1 tx per second
			// but you better to implement your own iterator in database, then you can send unlimited
			// but make sure id is less than 1 << 23, when it is higher start from 0 again
			return uint32(createdAt % (1 << 23)), createdAt, nil
		},
	})
	if err != nil {
		log.Fatalln("FromSeed err:", err.Error())
		return
	}

	log.Println("Wallet address:", w.WalletAddress())

	connStr := os.Getenv("DATABASE_URL")
	db, err = gorm.Open(postgres.Open(connStr), &gorm.Config{})
	if err != nil {
		log.Fatalln("Database connection error:", err.Error())
	}

	err = db.AutoMigrate(&Transaction{})
	if err != nil {
		log.Fatalln("Error creating table:", err.Error())
	}

	log.Println("Initialized database and table")

	// Получение токена бота из переменной окружения
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatalf("TELEGRAM_BOT_TOKEN not set in .env file")
	}

	// Создание нового бота
	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	chatID = os.Getenv("TELEGRAM_CHAT_ID")
	if chatID == "" {
		log.Fatalf("TELEGRAM_CHAT_ID not set in .env file")
	}

	// Отправка сообщения в чат
	messageText := "Payment service started"
	msg := tgbotapi.NewMessageToChannel(chatID, messageText)
	_, err = bot.Send(msg)
	if err != nil {
		log.Panic(err)
	}

	log.Println("Message sent successfully!")
}
