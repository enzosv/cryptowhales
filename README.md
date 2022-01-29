# What this is
A tool to scrape and report the balances of top cryptocurrency wallets

## Features
Displays historical balances for whales via [website]()
Reports balance summaries via [telegram channel]()
Stores historical data for other analysis
Distinguishes wallets based on analysis*

## Disclaimers
* Not financial advice
* *Work in progress. Not 100% sure about the way wallets are distinguished
  * Actually not 100% sure about anything at all. Feel free to contribute.
* Not all wallets and blockchains are captured

## TODO
* Better website
* Capture more data
* Consider timescale db for smaller footprint
* Rename cold wallets to accumulators and hot wallets to sellers

# How it is analyzed
* Wallets are considered cold wallets by default
  * They are marked as hot wallets if their highest balance in the last 30 days is higher than their latest balance
|                            | Suggests      | Bullish | Bearish |   |
|----------------------------|---------------|---------|---------|---|
| Crypto into exchange       | Selling       |         | ✅       |   |
| Crypto out of exchange     | Holding       | ✅       |         |   |
| Stablecoin into exchange   | Buying        | ✅       |         |   |
| Stablecoin out of exchange | End of buying |         | ✅       |   |
| Crypto into cold wallet    | Buying        | ✅       |         |   |
| Crypto out of cold wallet  | Selling       |         | ✅       |   |


# Building
## Requirements
go
config.json. See 
database. See latest dump in releases.
## Steps
```
go get -d
go build
./cryptowhales
```