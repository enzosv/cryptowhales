# What this is
A tool to scrape and report the balances of top cryptocurrency wallets

## Telegram Preview
![preview](https://github.com/enzosv/cryptowhales/blob/main/telegram.png)

## Features
* Displays historical balances for whales via [website](https://enzosv.github.io/cryptowhales)
* Reports balance summaries via [telegram channel](https://t.me/whalesummary/)
* Stores historical data for other analysis
* Distinguishes wallets based on analysis
* Summarizes data in USD

## Disclaimers
* Not financial advice
* Work in progress. Not 100% sure about the way wallets are distinguished
  * Actually not 100% sure about anything at all. Feel free to contribute.
* Cold vs hot wallet data won't be accurate until enough time has passed
* Not all wallets and blockchains are captured
* This uses html scraping to collect data. It can fail if the websites change.

## TODO
* Better website
* Capture more data
* Consider timescale db for smaller footprint
* Rename cold wallets to accumulators and hot wallets to sellers
* Merge with [Whale Summary](https://github.com/enzosv/whalesummary)

# How it is analyzed

**Event**  | **Suggests** | **Analysis**
---------- | ------------ | ------------ 
Crypto into exchange | Selling | `Bearish` 
Crypto out of exchange | Holding | **Bullish** 
Stablecoin into exchange | Buying | **Bullish** 
Stablecoin out of exchange | End of buying | `Bearish` 
Crypto into cold wallet | Buying | **Bullish** 
Crypto out of cold wallet | Selling | `Bearish` 
## Additional notes
* Telegram bot and website adds the total of these movements to the time header
    * There may be duplicates. i.e. A cold wallet transferring to an exchange.
* Wallets are considered cold wallets by default
  * They are marked as hot wallets if their highest balance in the last 30 days is higher than their latest balance


# Building
## Requirements
1. go
2. config.json. See [sample_config.json](https://github.com/enzosv/cryptowhales/blob/master/sample_config.json). 
3. database. See latest dump in releases.
## Steps
```
go get -d
go build
./cryptowhales
```
# Credits
* Inspired by [WhaleStats](https://www.whalestats.com/) and [WhaleAlert](https://whale-alert.io/)
* Data sourced from [etherscan](https://etherscan.io/accounts), [bitinfocharts](https://bitinfocharts.com/top-100-richest-bitcoin-addresses.html) and [coingecko](https://www.coingecko.com/)
* [pgx](https://github.com/jackc/pgx) used as the database driver
* [goquery](https://github.com/PuerkitoBio/goquery) used for scraping
* [golang-migrate](https://github.com/golang-migrate/migrate) used to generate migration files

Tips are appreciated. 0xBa2306a4e2AadF2C3A6084f88045EBed0E842bF9