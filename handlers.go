package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func testHandler(responseWriter http.ResponseWriter, req *http.Request) {
	fmt.Println("ok")
}

func sendTransactionsHandler(responseWriter http.ResponseWriter, req *http.Request) {
	payload, err := getPayload(req)
	if err != nil {
		respondError(responseWriter, err)
		return
	}
	if payload.ApiKey != os.Getenv("API_KEY") {
		respondError(responseWriter, fmt.Errorf("Unauthorized!"))
		return
	}

	if len(payload.Txs) == 0 {
		respondError(responseWriter, fmt.Errorf("No transactions!"))
		return
	}

	for _, tx := range payload.Txs {
		tx.Status = "Wait"
		if err := db.Create(&tx).Error; err != nil {
			respondError(responseWriter, fmt.Errorf("Database error: %v", err))
			return
		}
	}

	responseWriter.WriteHeader(http.StatusOK)
	json.NewEncoder(responseWriter).Encode(map[string]string{"status": "success"})
}

func getPayload(req *http.Request) (*RequestPayload, error) {
	var payload RequestPayload
	err := json.NewDecoder(req.Body).Decode(&payload)

	if err != nil {
		log.Println("Json decode err:", err.Error())
		return nil, err
	}

	log.Println("Transactions:", payload.Txs)
	return &payload, nil
}

func respondError(responseWriter http.ResponseWriter, err error) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": err.Error(),
	})
}
