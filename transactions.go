package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton/jetton"
	"github.com/xssnick/tonutils-go/ton/wallet"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"gorm.io/gorm"
	"log"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Transaction struct {
	gorm.Model
	UserId  uint64 `json:"userId"`
	Address string `json:"address"`
	Amount  string `json:"amount"`
	Status  string `json:"status"`
}

type RequestPayload struct {
	ApiKey string        `json:"apiKey"`
	Txs    []Transaction `json:"txs"`
}

func processPendingTransactions() {
	for {
		time.Sleep(5 * time.Second) // adjust the interval as needed
		var txs []Transaction
		if err := db.Where("status = ?", "Wait").
			Or("status = ?", "NeedRetry").
			Limit(MAX_CONCURRENT_TXS).
			Find(&txs).Error; err != nil {
			log.Println("Database query error:", err)
			continue
		}

		if len(txs) == 0 {
			continue
		}

		err := processTransactions(context.Background(), "", txs)
		if err != nil {
			log.Println("Processing transactions error:", err)
			continue
		}
	}
}

var errMsgAllreadySended = false

func processTransactions(ctx context.Context, commentText string, txs []Transaction) error {
	setStatus(txs, "Pending")
	block, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}

	balance, err := w.GetBalance(ctx, block)
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}
	decimals, err := getDecimalsFromEnv()
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}
	totalAmount, err := calculateTotalAmount(txs, decimals)
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}
	// Initialize token wallet
	token := jetton.NewJettonMasterClient(api, address.MustParseAddr(os.Getenv("JETTON_MASTER")))
	jettonWallet, err := token.GetJettonWallet(ctx, w.WalletAddress())
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}

	jettonBalance, err := jettonWallet.GetBalance(ctx)
	if err != nil {
		setStatus(txs, "NeedRetry")
		return err
	}

	fmt.Printf("Balance: %s jettons\n", jettonBalance)
	if jettonBalance.Uint64() < totalAmount {
		setStatus(txs, "NeedRetry")
		msgText := "no USDT"
		if !errMsgAllreadySended {
			errMsgAllreadySended = true
			msg := tgbotapi.NewMessageToChannel(chatID, msgText)
			_, _ = bot.Send(msg)
		}
		return fmt.Errorf(msgText)
	}
	errMsgAllreadySended = false
	msgFeeStr := os.Getenv("MSG_FEE")
	msgFee, err := strconv.ParseFloat(msgFeeStr, 64)
	if err != nil {
		return fmt.Errorf("invalid value for MSG_FEE: %v", err)
	}
	msgFeeNano := uint64(msgFee * 1e9) // Convert to nanocoins

	if balance.Nano().Uint64() >= uint64(len(txs))*msgFeeNano {
		comment, err := wallet.CreateCommentCell(commentText)
		if err != nil {
			setStatus(txs, "NeedRetry")
			return err
		}

		messages := createMessages(jettonWallet, comment, txs, decimals)

		log.Println("Sending transaction and waiting for confirmation...")

		txHash, err := w.SendManyWaitTxHash(ctx, messages)
		if err != nil {
			setStatus(txs, "MaybeError")
			msgText := "Tx processing warning. Maybe error."
			msg := tgbotapi.NewMessageToChannel(chatID, msgText)
			_, _ = bot.Send(msg)
			return err
		}
		setStatus(txs, "Complete")

		log.Println("Transaction sent, hash:", base64.StdEncoding.EncodeToString(txHash))
		log.Println("Explorer link: https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash))

		return nil
	}
	msgText := "not enough balance to cover the transaction fees"
	msg := tgbotapi.NewMessageToChannel(chatID, msgText)
	_, _ = bot.Send(msg)
	return fmt.Errorf(msgText)
}

func getDecimalsFromEnv() (int, error) {
	decimalsStr := os.Getenv("DECIMALS")
	if decimalsStr == "" {
		return 0, fmt.Errorf("DECIMALS environment variable is not set")
	}
	decimals, err := strconv.Atoi(decimalsStr)
	if err != nil {
		return 0, fmt.Errorf("invalid value for DECIMALS: %v", err)
	}
	return decimals, nil
}

func calculateTotalAmount(txs []Transaction, decimals int) (uint64, error) {
	var total uint64
	for _, tx := range txs {
		amount, err := strconv.ParseFloat(tx.Amount, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid amount %s for address %s", tx.Amount, tx.Address)
		}
		total += uint64(amount * float64(pow10(decimals)))
	}
	return total, nil
}

func pow10(n int) int64 {
	result := int64(1)
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}

func createMessages(jettonWallet *jetton.WalletClient, comment *cell.Cell, txs []Transaction, decimals int) []*wallet.Message {
	var messages []*wallet.Message
	for _, tx := range txs {
		log.Printf("Address: %s, Amount: %s\n", tx.Address, tx.Amount)
		amountTokens := tlb.MustFromDecimal(tx.Amount, decimals)
		to := address.MustParseAddr(tx.Address)
		transferPayload, err := jettonWallet.BuildTransferPayloadV2(to, to, amountTokens, tlb.ZeroCoins, comment, nil)
		if err != nil {
			log.Fatalf("Error building transfer payload: %v", err)
		}
		walletMsg := wallet.SimpleMessage(jettonWallet.Address(), tlb.MustFromTON(os.Getenv("MSG_FEE")), transferPayload)
		walletMsg.Mode = 0
		messages = append(messages, walletMsg)
	}
	return messages
}

func setStatus(txs []Transaction, status string) {
	for _, tx := range txs {
		tx.Status = status
		if err := db.Save(&tx).Error; err != nil {
			log.Println("Database update error:", err)
		}
	}
}
