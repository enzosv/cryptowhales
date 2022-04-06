package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type TokenContract struct {
	Symbol     string `json:"symbol"`
	Address    string `json:"address"`
	Blockchain string `json:"blockchain"`
}

type Wallet struct {
	Blockchain string
	Address    string
	Name       string
	Balance    float64
	Percentage float64
	IsContract bool
	OwnerType  string
	Symbol     string
}

type Series struct {
	Date              int64   `json:"-"`
	Exchange          float64 `json:"exchange,omitempty"`
	Wrap              float64 `json:"wrap,omitempty"`
	Stake             float64 `json:"stake,omitempty"`
	DiamondHands      float64 `json:"diamond_hands,omitempty"`
	PaperHands        float64 `json:"paper_hands,omitempty"`
	DiamondHandsCount int     `json:"diamond_hands_count,omitempty"`
	PaperHandsCount   int     `json:"paper_hands_count,omitempty"`
}

type Config struct {
	Telegram TelegramConfig  `json:"telegram"`
	Database string          `json:"pg_url"`
	Output   string          `json:"output"`
	Tokens   []TokenContract `json:"tokens"`
}

type TelegramConfig struct {
	BotID       string `json:"bot_id"`
	RecipientID string `json:"recipient_id"`
}

type Point struct {
	Date int64  `json:"date"`
	Eth  Series `json:"eth,omitempty"`
	Btc  Series `json:"btc,omitempty"`
	USD  Series `json:"usd,omitempty"`
}

type contextKey int
type chainID int
type Blockchain struct {
	ID    chainID
	Price float64
}

const (
	chain contextKey = iota
)
const (
	Bitcoin chainID = iota
	Ethereum
)

func (b Blockchain) name() string {
	return []string{"bitcoin", "ethereum"}[b.ID]
}

func (b Blockchain) symbol() string {
	return []string{"BTC", "ETH"}[b.ID]
}

const TGURL = "https://api.telegram.org"

func main() {
	start := time.Now().Unix()
	configPath := flag.String("c", "config.json", "config file")
	shouldUpdate := flag.Bool("update", false, "flag to trigger batch update")
	flag.Parse()
	config := parseConfig(*configPath)

	if config.Database == "" {
		fmt.Println("provide db")
		return
	}

	defer fmt.Printf("execution took %d seconds\n", time.Now().Unix()-start)

	ctx := context.Background()
	blockchains := []Blockchain{{Bitcoin, 0}, {Ethereum, 0}}
	if *shouldUpdate {
		fmt.Println("updating")
		err := batchUpdate(ctx, config.Database, blockchains, config.Tokens)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	pricedChains, err := fetchPrice(blockchains)
	if err != nil {
		fmt.Println(err)
		return
	}

	points, err := generatePoints(ctx, config.Database)
	if err != nil {
		fmt.Println(err)
		return
	}

	logPrice(pricedChains, "price.json")

	if config.Telegram.BotID != "" && config.Telegram.RecipientID != "" {
		message := summarize(points, pricedChains)
		var priceMessage []string
		for _, c := range pricedChains {
			// TODO: add price change
			priceMessage = append(priceMessage, fmt.Sprintf("%s: %.1fK", c.symbol(), c.Price/1000))
		}
		message = fmt.Sprintf("[%s](https://enzosv.github.io/cryptowhales)\n\n%s", strings.Join(priceMessage, ", "), message)
		if message != "" {
			sendMessage(config.Telegram.BotID, config.Telegram.RecipientID, message)
		}
	}

	if config.Output == "" || !*shouldUpdate {
		return
	}
	latest, err := json.Marshal(points)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile(config.Output, latest, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func logPrice(pricedChains []Blockchain, path string) error {
	// only concerned with latest price
	file, err := json.Marshal(pricedChains)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, file, 0644)
}

func generatePoints(ctx context.Context, pg_url string) ([]Point, error) {
	conn, err := pgx.Connect(ctx, pg_url)
	if err != nil {
		return nil, err
	}
	ethseries, err := generate_eth_series(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("generate eth series error: %w", err)
	}
	btcseries, err := generate_btc_series(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("generate btc series error: %w", err)
	}
	usdseries, err := generate_usd_series(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("generate usd series error: %w", err)
	}
	var points []Point
	// assumes eth has the all the dates
	for _, p := range ethseries {
		var point Point
		point.Eth = p
		point.Date = p.Date
		points = append(points, point)
	}
	// TODO: Convert to map[date]series
	for i, p := range points {
		for _, b := range btcseries {
			if b.Date != p.Date {
				continue
			}
			points[i].Btc = b
			break
		}
		for _, u := range usdseries {
			if u.Date != p.Date {
				continue
			}
			points[i].USD = u
			break
		}
	}
	return points, nil
}

func commit(ctx context.Context, tx pgx.Tx, batch *pgx.Batch) error {
	results := tx.SendBatch(ctx, batch)
	err := results.Close()
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			return fmt.Errorf("error in sending batch (%s): %s. Hint: %s. (detail: %s, type: %s) where: line %d position %d in routine %s - %w", pgerr.Code, pgerr.Message, pgerr.Hint, pgerr.Detail, pgerr.DataTypeName, pgerr.Line, pgerr.Position, pgerr.Routine, err)
		}
		return fmt.Errorf("error in sending batch: %w", err)
	}
	return tx.Commit(ctx)
}

func btcUpdate(ctx context.Context, conn *pgxpool.Pool) error {
	var wallets []Wallet
	for i := 0; i < 40; i++ {
		ws, err := scrapeBTC(i+1, 10, 300*time.Millisecond)
		if err != nil {
			return err
		}
		for i, wallet := range wallets {
			for _, w := range ws {
				if wallet.Address == w.Address {
					// remove eariler duplicate
					wallets[i] = Wallet{}
				}
			}
		}
		wallets = append(wallets, ws...)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	batch := &pgx.Batch{}
	logScrape(batch, wallets)
	err = commit(ctx, tx, batch)
	if err != nil {
		return err
	}
	return nil
}

func ethUpdate(ctx context.Context, conn *pgxpool.Pool, tokens []TokenContract) error {
	var wallets []Wallet
	for i := 0; i < 100; i++ {
		ws, err := scrapeEth(i+1, 10, 300*time.Millisecond)
		if err != nil {
			return err
		}
		for i, wallet := range wallets {
			for _, w := range ws {
				if wallet.Address == w.Address {
					// remove eariler duplicate
					wallets[i] = Wallet{}
				}
			}
		}
		wallets = append(wallets, ws...)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	batch := &pgx.Batch{}

	logScrape(batch, wallets)

	err = commit(ctx, tx, batch)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		batch := &pgx.Batch{}
		var twallets []Wallet
		for i := 0; i < 20; i++ {
			ws, err := scrapeEthToken(token, i+1, 10, 300*time.Millisecond)
			if err != nil {
				return err
			}
			for i, wallet := range twallets {
				for _, w := range ws {
					if wallet.Address == w.Address {
						// remove eariler duplicate
						twallets[i] = Wallet{}
					}
				}
			}
			twallets = append(twallets, ws...)
		}
		logScrape(batch, twallets)

		err = commit(ctx, tx, batch)
		if err != nil {
			return err
		}
	}
	return nil
}

func batchUpdate(pctx context.Context, pg_url string, blockchains []Blockchain, tokens []TokenContract) error {
	pool, err := pgxpool.Connect(pctx, pg_url)
	if err != nil {
		return err
	}
	defer pool.Close()
	// do btc and eth simultaniously
	eg := new(errgroup.Group)
	for _, blockchain := range blockchains {

		if blockchain.ID == Bitcoin {
			eg.Go(func() error {
				// do btc in background
				ctx := context.WithValue(pctx, chain, blockchain.ID)
				return btcUpdate(ctx, pool)
			})
		}
		if blockchain.ID == Ethereum {
			eg.Go(func() error {
				// do eth in background
				ctx := context.WithValue(pctx, chain, blockchain.ID)
				var eth_tokens []TokenContract
				for _, token := range tokens {
					if token.Blockchain == blockchain.name() {
						eth_tokens = append(eth_tokens, token)
					}
				}
				return ethUpdate(ctx, pool, eth_tokens)
			})
		}
	}

	return eg.Wait()
}

func getDoc(url string, retries int, wait time.Duration) (*goquery.Document, error) {
	time.Sleep(wait)
	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		if retries > 0 {
			return getDoc(url, retries-1, wait)
		}
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		fmt.Println("status code error")
		if retries > 0 {
			return getDoc(url, retries-1, wait)
		}
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		fmt.Println(err)
		if retries > 0 {
			return getDoc(url, retries-1, wait)
		}
		return nil, err
	}
	return doc, nil
}

func fetchPrice(chains []Blockchain) ([]Blockchain, error) {
	var ids []string
	for _, chain := range chains {
		ids = append(ids, chain.name())
	}
	request_url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", strings.Join(ids, ","))
	res, err := http.Get(request_url)
	if err != nil {
		return chains, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return chains, err
	}
	var result map[string]map[string]float64
	err = json.Unmarshal(body, &result)
	if err != nil {
		return chains, err
	}

	var pricedChains []Blockchain
	for _, chain := range chains {
		pricedChains = append(pricedChains, Blockchain{chain.ID, result[chain.name()]["usd"]})
	}
	return pricedChains, nil
}

// scrape

// TODO: Create scrape protocol
func scrapeEthToken(token TokenContract, page, retries int, wait time.Duration) ([]Wallet, error) {
	url := fmt.Sprintf("https://etherscan.io/token/generic-tokenholders2?a=%s&p=%d", token.Address, page)
	doc, err := getDoc(url, retries, wait)
	if err != nil {
		return nil, err
	}
	var wallets []Wallet
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		var wallet Wallet
		wallet.Symbol = token.Symbol
		wallet.Blockchain = token.Blockchain
		s.Find("td").Each(func(j int, ss *goquery.Selection) {

			text := strings.ReplaceAll(ss.Text(), " ", "")

			switch j {
			case 1:
				html, _ := ss.Html()
				wallet.IsContract = strings.Contains(html, "Contract")
				if strings.HasPrefix(text, "0x") {
					wallet.Address = text
				} else {
					wallet.Name = text
					split := strings.Split(html, fmt.Sprintf("%s?a=", token.Address))
					split = strings.Split(split[1], " ")
					wallet.Address = strings.TrimSuffix(split[0], `"`)
				}

			case 2:
				bal := strings.ReplaceAll(text, ",", "")
				f, err := strconv.ParseFloat(bal, 64)
				if err != nil {
					fmt.Println(err)
				} else {
					wallet.Balance = f
				}
			}
		})
		// would have to manually edit on db if these assumptions are inaccurate
		if wallet.Balance > 0 {
			ownertype := "unknown"
			if wallet.IsContract {
				ownertype = "contract"
			} else if wallet.Name != "" {
				ownertype = "exchange"
			}
			wallet.OwnerType = ownertype
			wallets = append(wallets, wallet)
		}
	})
	if len(wallets) < 50 && retries > 0 {
		return scrapeEthToken(token, page, retries-1, wait)
	}
	fmt.Println(url)
	return wallets, nil
}

func scrapeBTC(page, retries int, wait time.Duration) ([]Wallet, error) {

	url := fmt.Sprintf("https://bitinfocharts.com/top-100-richest-bitcoin-addresses-%d.html", page)
	doc, err := getDoc(url, retries, wait)
	if err != nil {
		return nil, err
	}
	var wallets []Wallet
	tables := []string{"#tblOne", "#tblOne2"}
	for _, table := range tables {
		doc.Find(table).Find("tr").Each(func(i int, s *goquery.Selection) {
			var wallet Wallet
			// TODO: Avoid hardcode
			wallet.Symbol = "BTC"
			wallet.Blockchain = "bitcoin"
			wallet.IsContract = false
			s.Find("td").Each(func(j int, ss *goquery.Selection) {
				text := strings.ReplaceAll(ss.Text(), " ", "")
				switch j {
				case 1:
					//Wallet
					ss.Find("a").Each(func(k int, sss *goquery.Selection) {
						t := strings.ReplaceAll(sss.Text(), " ", "")
						switch k {
						case 0:
							wallet.Address = t
						case 1:
							wallet.Name = t
						}
					})
				case 2:
					//balance
					bal := strings.Split(text, "BTC")[0]
					bal = strings.ReplaceAll(bal, "BTC", "")
					bal = strings.ReplaceAll(bal, ",", "")
					f, err := strconv.ParseFloat(bal, 64)
					if err != nil {
						fmt.Println(err)
					} else {
						wallet.Balance = f
					}
				}
			})
			// would have to manually edit on db if these assumptions are inaccurate
			if wallet.Balance > 0 {
				ownertype := "unknown"
				if wallet.Name != "" {
					ownertype = "exchange"
				}
				wallet.OwnerType = ownertype
				wallets = append(wallets, wallet)
			}
		})
	}

	if len(wallets) < 100 && retries > 0 {
		return scrapeBTC(page, retries-1, wait)
	}
	fmt.Println(url)
	return wallets, nil
}

func scrapeEth(page, retries int, wait time.Duration) ([]Wallet, error) {
	url := fmt.Sprintf("https://etherscan.io/accounts/%d?ps=100", page)
	doc, err := getDoc(url, retries, wait)
	if err != nil {
		return nil, err
	}
	var wallets []Wallet
	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		var wallet Wallet
		// TODO: Avoid hardcode
		wallet.Symbol = "ETH"
		wallet.Blockchain = "ethereum"
		s.Find("td").Each(func(j int, ss *goquery.Selection) {

			text := strings.ReplaceAll(ss.Text(), " ", "")
			switch j {
			case 1:
				html, _ := ss.Html()
				wallet.IsContract = strings.Contains(html, "Contract")
				wallet.Address = text
			case 2:
				wallet.Name = text
			case 3:
				bal := strings.ReplaceAll(text, "Ether", "")
				bal = strings.ReplaceAll(bal, ",", "")
				f, err := strconv.ParseFloat(bal, 64)
				if err != nil {
					fmt.Println(err)
				} else {
					wallet.Balance = f
				}
				// case 4:
				// percentage := strings.ReplaceAll(text, "%", "")
				// f, err := strconv.ParseFloat(percentage, 64)
				// if err != nil {
				// 	log.Fatal(err)
				// }
				// wallet.Percentage = f
			}
		})
		// would have to manually edit on db if these assumptions are inaccurate
		if wallet.Balance > 0 {
			ownertype := "unknown"
			if wallet.IsContract {
				ownertype = "contract"
			} else if wallet.Name != "" {
				ownertype = "exchange"
			}
			wallet.OwnerType = ownertype
			wallets = append(wallets, wallet)
		}
	})
	if len(wallets) < 100 && retries > 0 {
		return scrapeEth(page, retries-1, wait)
	}
	fmt.Println(url)
	return wallets, nil
}

func logScrape(batch *pgx.Batch, wallets []Wallet) {
	query := `
		INSERT INTO whale
		(blockchain, address, owner, owner_type, is_contract)
		VALUES ($5, $1, NULLIF($2, ''), $3, $4)
		ON CONFLICT ON CONSTRAINT ux_blockchain_address DO UPDATE
		SET owner = NULLIF($2, '');
	`
	balquery := `
		INSERT INTO balance
		(whale_id, value, symbol)
		VALUES (
			(SELECT whale_id FROM whale WHERE address = $1 AND blockchain = $4), 
			$2, $3
		);
	`

	for _, wallet := range wallets {
		if wallet.Balance <= 0 {
			continue
		}
		batch.Queue(query, wallet.Address, wallet.Name, wallet.OwnerType, wallet.IsContract, wallet.Blockchain)
		batch.Queue(balquery, wallet.Address, wallet.Balance, wallet.Symbol, wallet.Blockchain)
	}
}

// generate series
func generate_usd_series(ctx context.Context, conn *pgx.Conn) ([]Series, error) {
	query := `
	select 
		coalesce(sum(b.value), 0) as exchange,
		extract(epoch from date_trunc('hour', b.created_at)) as epoch
	from balance b
	join whale w using(whale_id)
	where 
		w.owner_type = 'exchange'
		AND b.symbol like '%USD%' 
		AND date_trunc('hour', b.created_at) >= to_timestamp(1641744000.000000)  --ignore values before full capture
		AND date_trunc('hour', b.created_at) > now()-'31 days'::interval
	group by  date_trunc('hour', b.created_at)
	order by date_trunc('hour', b.created_at)
	;
	`
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()
	var data []Series
	for rows.Next() {
		if rows.Err() != nil {
			return nil, fmt.Errorf("row error: %w", err)
		}
		var row Series
		err := rows.Scan(
			&row.Exchange, &row.Date,
		)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		data = append(data, row)
	}
	return data, nil
}

func generate_btc_series(ctx context.Context, conn *pgx.Conn) ([]Series, error) {
	query := `
	select 
		coalesce(sum(b.value) filter (where w.owner_type = 'exchange'), 0) as exchange,
		sum(b.value) filter (where coalesce(b2.value, 0) <= b.value and not w.owner_type = 'exchange') as diamond_hands,
		coalesce(sum(b.value) filter (where b2.value > b.value +1 and not w.owner_type = 'exchange'),0) as paper_hands,
		count(b.value) filter (where coalesce(b2.value, 0) <= b.value and not w.owner_type = 'exchange') as diamond_hands_count,
		coalesce(count(b.value) filter (where b2.value > b.value +1 and not w.owner_type = 'exchange'),0) as paper_hands_count,
		extract(epoch from date_trunc('hour', b.created_at)) as epoch
	from balance b
	join whale w using(whale_id)
	left outer join (
		select b.whale_id, max(b.value) as value, min(b.created_at) as created_at
		from balance b
		INNER JOIN (
			SELECT whale_id, MAX(value) as value 
			FROM balance b
			WHERE b.symbol = 'BTC'
			AND b.created_at < now()-'1 hour'::interval 
			AND b.created_at > now()-'61 days'::interval
			GROUP BY whale_id
		) l ON b.whale_id = l.whale_id AND b.value = l.value
		WHERE b.symbol = 'BTC' 
		group by b.whale_id
	) b2 
	on b2.whale_id = b.whale_id 
		and b2.created_at <  b.created_at
		--and b2.created_at > b.created_at-'31 days'::interval
	where b.created_at > now()-'31 days'::interval
	and b.symbol = 'BTC'
	and w.blockchain = 'bitcoin'
	group by  date_trunc('hour', b.created_at)
	order by date_trunc('hour', b.created_at)
	;
	`
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()
	var data []Series
	for rows.Next() {
		if rows.Err() != nil {
			return nil, fmt.Errorf("row error: %w", err)
		}
		var row Series
		err := rows.Scan(
			&row.Exchange,
			&row.DiamondHands, &row.PaperHands,
			&row.DiamondHandsCount, &row.PaperHandsCount,
			&row.Date)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		data = append(data, row)
	}
	return data, nil
}

func generate_eth_series(ctx context.Context, conn *pgx.Conn) ([]Series, error) {
	query := `
	select 
		coalesce(sum(b.value) filter (where w.owner_type = 'exchange'), 0) as exchange,
		coalesce(sum(b.value) filter (where w.owner_type = 'wrap'), 0) as wrap,
		coalesce(sum(b.value) filter (where w.owner_type = 'stake'), 0) as stake,
		sum(b.value) filter (
			where coalesce(b2.value, 0) <= b.value 
			and not w.is_contract 
			and w.owner_type not in ('exchange', 'stake', 'wrap', 'burn')
		) as diamond_hands,
		coalesce(sum(b.value) filter (where b2.value > b.value +1),0) as paper_hands,
		count(b.value) filter (
			where coalesce(b2.value, 0) <= b.value 
			and not w.is_contract 
			and w.owner_type not in ('exchange', 'stake', 'wrap', 'burn')
		) as diamond_hands_count,
		coalesce(count(b.value) filter (where b2.value > b.value +1),0) as paper_hands_count,
		extract(epoch from date_trunc('hour', b.created_at)) as epoch
	from balance b
	join whale w using(whale_id)
	left outer join (
		select b.whale_id, max(b.value) as value, min(b.created_at) as created_at
		from balance b
		INNER JOIN (
			SELECT b.whale_id, MAX(value) as value 
			FROM balance b
			join whale w using (whale_id)
			where not w.is_contract 
			and w.blockchain = 'ethereum'
			and not w.owner_type in ('exchange', 'stake', 'wrap', 'burn')
			and b.symbol = 'ETH'
			and b.created_at < now()-'1 hour'::interval 
			AND b.created_at > now()-'61 days'::interval
			GROUP BY b.whale_id
			having max(value) > min(value) --ignore illiquid
		) l ON b.whale_id = l.whale_id AND b.value = l.value
		group by b.whale_id
	) b2 
	on b2.whale_id = b.whale_id 
		and b2.created_at < b.created_at
		--and b2.created_at >= b.created_at-'31 days'::interval
	where 
		b.symbol = 'ETH' 
		and not w.owner_type = 'burn'
		AND b.created_at > now()-'31 days'::interval
		and w.blockchain = 'ethereum'
	group by  date_trunc('hour', b.created_at)
	order by date_trunc('hour', b.created_at)
	;
	`
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()
	var data []Series
	for rows.Next() {
		if rows.Err() != nil {
			return nil, fmt.Errorf("row error: %w", err)
		}
		var row Series
		err := rows.Scan(
			&row.Exchange, &row.Wrap, &row.Stake,
			&row.DiamondHands, &row.PaperHands,
			&row.DiamondHandsCount, &row.PaperHandsCount,
			&row.Date)
		if err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		data = append(data, row)
	}
	return data, nil
}

func summarize(points []Point, blockchains []Blockchain) string {
	var differences []string
	milestones := map[string]int{
		"1h":  1,
		"4h":  4,
		"24h": 24,
		"7d":  168,
		"30d": 720,
	}
	// map iteration is random. force this order
	keys := []string{"1h", "4h", "24h", "7d", "30d"}
	latest := points[len(points)-1]
	p := message.NewPrinter(language.English)
	for _, k := range keys {
		m := milestones[k]
		if len(points) < 1+m {
			continue
		}
		msg := []string{fmt.Sprintf("*%s*:", k)}
		point := points[len(points)-(1+m)]
		overall := 0.0
		for _, blockchain := range blockchains {
			var new Series
			var old Series
			switch blockchain.ID {
			case Bitcoin:
				new = latest.Btc
				old = point.Btc
			case Ethereum:
				new = latest.Eth
				old = point.Eth
			}
			m, sum := analyze(new, old, blockchain.symbol(), false)
			msg = append(msg, m...)
			overall += sum * blockchain.Price
		}
		// TODO: Avoid hardcode
		usdMessage, usdSum := analyze(latest.USD, point.USD, "USD", true)
		msg = append(msg, usdMessage...)
		overall += usdSum
		var dif string
		abs := math.Abs(overall)
		if abs >= 1000000000 {
			dif = p.Sprintf("%.2fB", abs/1000000000)
		} else if abs >= 1000000 {
			dif = p.Sprintf("%.2fM", abs/1000000)
		} else if abs >= 1000 {
			dif = p.Sprintf("%.2fK", abs/1000)
		}
		if overall > 0 {
			msg[0] = fmt.Sprintf("*%s*: *+$%s*", k, dif)
		} else if overall < 0 {
			msg[0] = fmt.Sprintf("*%s*: `-$%s`", k, dif)
		}
		differences = append(differences, msg...)
	}
	return strings.Join(differences, "\n")
}

func analyze(now, old Series, symbol string, is_stablecoin bool) ([]string, float64) {
	if old.Exchange == 0 {
		// assume empty if exchange is empty
		return nil, 0
	}
	// diamond := (now.DiamondHands - old.DiamondHands) * 100 / ((now.DiamondHands + old.DiamondHands) / 2)
	// exchange := (now.Exchange - old.Exchange) * 100 / ((now.Exchange + old.Exchange) / 2)
	// stake := (now.Stake - old.Stake) * 100 / ((now.Stake + old.Stake) / 2)

	var odividend float64
	var odivisor float64

	overall := (now.DiamondHands - old.DiamondHands) + (now.Stake - old.Stake)
	if is_stablecoin {
		odividend = 100 * ((now.DiamondHands + now.Stake - now.Exchange) - (old.DiamondHands + old.Stake - old.Exchange))
		odivisor = ((now.DiamondHands + now.Stake - now.Exchange) + (old.DiamondHands + old.Stake - old.Exchange)) / 2
		overall += (now.Exchange - old.Exchange)
	} else {
		odividend = 100 * ((now.DiamondHands + now.Stake + now.Exchange) - (old.DiamondHands + old.Stake + old.Exchange))
		odivisor = ((now.DiamondHands + now.Stake + now.Exchange) + (old.DiamondHands + old.Stake + old.Exchange)) / 2
		overall -= (now.Exchange - old.Exchange)
	}
	odif := odividend / odivisor

	var msg []string
	// if math.Abs(diamond) >= 0.1 {
	// 	value := fmt.Sprintf("`%.2f%%`", diamond)
	// 	if diamond > 0 {
	// 		// whales stacking crypto is a positive
	// 		value = fmt.Sprintf("*+%.2f%%*", diamond)
	// 	}
	// 	msg = append(msg, fmt.Sprintf("\t`[%s]` `%-12s`: %s", symbol, "Cold Wallets", value))
	// }
	// if math.Abs(stake) >= 0.1 {
	// 	value := fmt.Sprintf("`%.2f%%`", stake)
	// 	if stake > 0 {
	// 		// whales locking crypto is a positive
	// 		value = fmt.Sprintf("*+%.2f%%*", stake)
	// 	}
	// 	msg = append(msg, fmt.Sprintf("\t`[%s]` `%-12s`: %s", symbol, "Staked", value))
	// }
	// if math.Abs(exchange) >= 0.1 {
	// 	var value string
	// 	if is_stablecoin {
	// 		if exchange > 0 {
	// 			// stablecoin entering exchanges is a positive
	// 			value = fmt.Sprintf("*+%.2f%%*", exchange)
	// 		} else if exchange < 0 {
	// 			// stablecoin leaving exchanges is a negative
	// 			value = fmt.Sprintf("`%.2f%%`", exchange)
	// 		}
	// 	} else {
	// 		if exchange > 0 {
	// 			// crypto entering exchanges is a negative
	// 			value = fmt.Sprintf("`+%.2f%%`", exchange)
	// 		} else if exchange < 0 {
	// 			// crypto leaving exchanges is a positive
	// 			value = fmt.Sprintf("*%.2f%%*", exchange)
	// 		}
	// 	}
	// 	msg = append(msg, fmt.Sprintf("\t`[%s]` `%-12s`: %s", symbol, "Exchanges", value))
	// }
	if math.Abs(odif) >= 0.1 {
		var overallValue string
		if odif > 0 {
			overallValue = fmt.Sprintf("*+%.2f%%*", odif)
		} else if odif < 0 {
			overallValue = fmt.Sprintf("`%.2f%%`", odif)
		}
		msg = append(msg, fmt.Sprintf("\t`%s`: %s", symbol, overallValue))
	}

	return msg, overall
}

func constructPayload(chatID, message string) (*bytes.Reader, error) {
	payload := map[string]interface{}{}
	payload["chat_id"] = chatID
	payload["text"] = message
	payload["parse_mode"] = "markdown"

	jsonValue, err := json.Marshal(payload)
	return bytes.NewReader(jsonValue), err
}

func sendMessage(bot, chatID, message string) error {
	payload, err := constructPayload(chatID, message)
	if err != nil {
		fmt.Println(err)
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/bot%s/sendMessage", TGURL, bot), payload)
	if err != nil {
		fmt.Println(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println(string(body))
	return nil
}

func parseConfig(path string) Config {
	configFile, err := os.Open(path)
	if err != nil {
		log.Fatal("Cannot open server configuration file: ", err)
	}
	defer configFile.Close()

	dec := json.NewDecoder(configFile)
	var config Config
	if err = dec.Decode(&config); errors.Is(err, io.EOF) {
		//do nothing
	} else if err != nil {
		log.Fatal("Cannot load server configuration file: ", err)
	}
	return config
}
