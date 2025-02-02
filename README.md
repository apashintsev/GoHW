# Highload Wallet Server for TON

## Features
High load handling capabilities
Ability to send batches of up to 250 transactions per request.
- RESTful API for easy integration with other applications

## Installation
1. Clone the repository: `git clone https://github.com/your_username/your_repository.git`
1. Install dependencies: `go mod download`
1. Run the server on port 8888: `go run main.go`

## Usage
The server exposes a RESTful API with the following endpoint:

### `/sendTransactions`
- `POST /sendTransactions` - Send transactions to other TON users
### Request
Query parameter:

- `send_mode`: string value representing the send mode. It can have the following values:
    - `0` -	Ordinary message
    - `64` - Carry all the remaining value of the inbound message in addition to the value initially indicated in the new message
    - `128` - Carry all the remaining balance of the current smart contract instead of the value originally indicated in the message

### Request body:
- JSON serialized dictionary containing the address of the user as the key and amount in Tons as the value.

Example:
```json
{
  "apiKey": "kasdnflakfajfawkwfnalkngf",
  "txs": [
    { "address": "UQCIovrJ35J8Y47q-pipigDPToi5Nyav4HcL1t-xPyd2jUhT", "amount": "0.02" },
    { "address": "UQCIovrJ35J8Y47q-pipigDPToi5Nyav4HcL1t-xPyd2jUhT", "amount": "0.01" },
    { "address": "UQCIovrJ35J8Y47q-pipigDPToi5Nyav4HcL1t-xPyd2jUhT", "amount": "0.03" }
  ]
}
```
