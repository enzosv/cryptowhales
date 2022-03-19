function populateTable(series) {
    function seriesDif(milestones, data, id, color) {
        var snow = data[data.length - 1][1]
        var html = '<td class="border px-8">' + id + '</td>'
        milestones.forEach(milestone => {
            
            var value = ""
            var val
            if (data.length < 1 + milestone) {
                return
            }
            var old = data[data.length - (1 + milestone)][1]
            if (isNaN(old)) {
                html += '<td class="border px-8"> </td>'
                return
            }
            val = ((snow - old) * 100 / ((snow+old)/2))
            value = val.toFixed(2) + '%' 
            var textColor = "white"
            if(id.includes("USD") || id.includes("Cold Wallets") || id.includes("Staked")){
                if(val > 0) {
                    textColor = "green-600"
                } else if (val < 0){
                    textColor = "red-600"
                }
            } else if (id.includes("Exchanges")){
                if(val > 0) {
                    textColor = "red-600"
                } else if (val < 0){
                    textColor = "green-600"
                }
            }
            html += `<td class="border px-8 text-${textColor}"> ${value} </td>`
        
        })
        document.getElementById(id).innerHTML = html
    }
    var colors = Highcharts.getOptions().colors
    
    let milestones = [1,4,24,168,720]
    
    analyze(series, milestones)
    for (i = 0; i < series.length; i++) {
        if(document.getElementById(series[i].name) === null){
            continue
        }
        seriesDif(milestones, series[i].data, series[i].name, colors[i])
    }
}



function analyze(series, milestones) {
    var series_index = [0,2,4,5,7]
    for(var i=0; i<milestones.length; i++) {
        var overall = 0
        series_index.forEach(index => {
            var group = series[index]
            if (group.data.length >= 1 + milestones[i]) {
                var old = group.data[group.data.length - (1 + milestones[i])]
                if (old.length < 1 || isNaN(old[1])) {
                    return
                }
                var snow = group.data[group.data.length - 1][1]
                var value = snow - old[1]
                
                
                if(group.name.includes("Exchange") && !group.name.includes("USD")) {
                    value *= -1
                }
                overall += value
            }
        })
        var value = ""
        var color = ""
        let abs = Math.abs(overall)
        var dif = ""
        if (abs >= 1000000000) {
			dif = Highcharts.numberFormat(abs/1000000000, 2)+"B"
		} else if (abs >= 1000000) {
			dif = Highcharts.numberFormat(abs/1000000, 2)+"M"
		} else if (abs >= 1000) {
			dif = Highcharts.numberFormat(abs/1000, 2)+"K"
		}
        if(overall>0){
            value = "+$"+dif
            color = "text-green-500"
        } else if (overall <0) {
            value = "-$"+dif
            color = "text-red-500"
        }
        document.getElementById(milestones[i]+"h").className  = color
        document.getElementById(milestones[i]+"h").innerHTML = value
    }
}

var date_options = { "day": "2-digit", "year": "numeric", "month": "short", "hour": "numeric" }
function generateStats(whale, coingecko){
    var btcprice = coingecko.bitcoin.usd
    var ethprice = coingecko.ethereum.usd
    let now = whale[whale.length-1]
    let ethtotal = (now.eth.diamond_hands+now.eth.paper_hands+now.eth.wrap+now.eth.stake+now.eth.exchange)*ethprice
    let btctotal = (now.btc.diamond_hands+now.btc.paper_hands+now.btc.exchange)*btcprice
    var ethChange = ""
    if(coingecko.ethereum.usd_24h_change > 0){
        ethChange = ' <small style="color:green;">+'+Math.abs(coingecko.ethereum.usd_24h_change).toFixed(2)+'%</small>'
    } else {
        ethChange = ' <small style="color:red;">-'+Math.abs(coingecko.ethereum.usd_24h_change).toFixed(2)+'%</small>'
    }
    let ethCapture = '<br><small>Top 10,000 <a href="https://etherscan.io/accounts">wallets</a> own '+Highcharts.numberFormat(ethtotal/coingecko.ethereum.usd_market_cap*100, 2) + '% of marketcap</small>'
    + '<br><small>Cold Wallets: '+(now.eth.diamond_hands*ethprice*100/ethtotal).toFixed(2)+'%</small>'
    + '<br><small>Hot Wallets: '+(now.eth.paper_hands*ethprice*100/ethtotal).toFixed(2)+'%</small>'
    + '<br><small>Exchanges: '+(now.eth.exchange*ethprice*100/ethtotal).toFixed(2)+'%</small>'
    let btcCapture = '<br><small>Top 4,000 <a href="https://bitinfocharts.com/top-100-richest-bitcoin-addresses.html">wallets</a> own '+Highcharts.numberFormat(btctotal/coingecko.bitcoin.usd_market_cap*100, 2) + '% of marketcap</small>'
    + '<br><small>Cold Wallets: '+(now.btc.diamond_hands*btcprice*100/btctotal).toFixed(2)+'%</small>'
    + '<br><small>Hot Wallets: '+(now.btc.paper_hands*btcprice*100/btctotal).toFixed(2)+'%</small>'
    + '<br><small>Exchanges: '+(now.btc.exchange*btcprice*100/btctotal).toFixed(2)+'%</small>'
    var btcChange = ""
    if(coingecko.bitcoin.usd_24h_change > 0){
        btcChange = ' <small style="color:green;">+'+Math.abs(coingecko.bitcoin.usd_24h_change).toFixed(2)+'%</small>'
    } else {
        btcChange = ' <small style="color:red;">-'+Math.abs(coingecko.bitcoin.usd_24h_change).toFixed(2)+'%</small>'
    }
    document.getElementById("eth_stats").innerHTML = '<a href="https://www.coingecko.com/en/coins/ethereum">ETH</a>: $'+Highcharts.numberFormat(ethprice, 2)+ethChange+ethCapture
    document.getElementById("btc_stats").innerHTML = '<a href="https://www.coingecko.com/en/coins/ethereum">BTC</a>: $'+Highcharts.numberFormat(btcprice, 2)+btcChange+btcCapture
    document.getElementById("last_updated").innerHTML = "Last updated: " + new Date(now.date*1000).toLocaleDateString("en-US", date_options)
}

function generateSeries(whale, coingecko) {
    var series = [
        {
            type: 'line',
            name: '[ETH] Cold Wallets',
            data: [],
            visible: false
        },
        {
            type: 'line',
            name: '[ETH] Hot Wallets',
            data: [],
            visible: false
        },
        {
            type: 'line',
            name: '[ETH] Exchanges',
            data: [],
            visible: true
        },
        {
            type: 'line',
            name: '[ETH] Staked',
            data: [],
            visible: false
        },
        {
            type: 'line',
            name: '[USD*] Exchanges',
            data: [],
            visible: true
        },
        {
            type: 'line',
            name: '[BTC] Cold Wallets',
            data: [],
            visible: false
        },
        {
            type: 'line',
            name: '[BTC] Hot Wallets',
            data: [],
            visible: false
        },
        {
            type: 'line',
            name: '[BTC] Exchanges',
            data: [],
            visible: true
        },
    ]
    var btcprice = coingecko.bitcoin.usd
    var ethprice = coingecko.ethereum.usd
    whale.forEach(point => {
        var date = point.date * 1000
        series[0].data.push([date, point.eth.diamond_hands * ethprice])
        series[1].data.push([date, point.eth.paper_hands * ethprice])
        series[2].data.push([date, point.eth.exchange * ethprice])
        series[3].data.push([date, point.eth.stake * ethprice])
        series[4].data.push([date, point.usd.exchange])
        series[5].data.push([date, point.btc.diamond_hands * btcprice])
        series[6].data.push([date, point.btc.paper_hands * btcprice])
        series[7].data.push([date, point.btc.exchange * btcprice])
    })
    return series
}
async function main() {
    const [whaleResponse, coingeckoResponse] = await Promise.all([
        fetch('https://enzosv.xyz/static/ethwhales.json'),
        fetch('https://api.coingecko.com/api/v3/simple/price?ids=bitcoin%2Cethereum&vs_currencies=usd&include_market_cap=true&include_24hr_change=true')
    ]);
    const whale = await whaleResponse.json();
    const coingecko = await coingeckoResponse.json();
    let series = generateSeries(whale, coingecko)
    generateStats(whale, coingecko)
    Highcharts.chart('container', {
        title: {
            text: 'Crypto whale balances',
            align: 'left'
        },
        tooltip: {
            formatter: function () {
                // var colors = Highcharts.getOptions().colors
                var tip = ['<b>' + new Date(this.x).toLocaleDateString("en-US", date_options) + '</b>']
                for (var i = 0; i < series.length; i++) {
                    var point = this.points[i]
                    if (point) {
                        tip.push('<div>' + point.series.name + '</div>: $' + Highcharts.numberFormat(point.y / 1000000000, 2) + 'B')
                    }
                }
                return tip.join('<br/>');

            },
            shared: true
        },
        series: series
    });
    populateTable(series);
    fetch("https://api.alternative.me/fng/").then(response => response.json())
    .then(response => {
        let data = response.data[0]
        console.log(data)
        document.getElementById("fng").innerHTML = data.value_classification + " ("+data.value+")"
    })
}
main()
Highcharts.setOptions({
    // colors: ['#0288D1', '#7F45F0', '#F1264C', '#B0DB43', '#F58F29', '#64E572',
    //          '#FF9655', '#FFF263', '#6AF9C4'],
    chart: {
        // backgroundColor: 'rgb(36, 52, 71)',
        // backgroundColor: '#111',
        marginLeft: 50,
        marginRight: 20,
        zoomType: 'x',
        reflow: true
    },
    credits: {
        enabled: false
    },

    xAxis: {
        type: 'datetime'
    },
    title: {
        style: {
            color: '#fff',
            font: 'bold 16px'
        }
    },
    plotOptions: {
        series: {
            marker: {
                enabled: false,
                states: {
                    hover: {
                        enabled: true,
                        radius: 3
                    }
                }
            }
        }
    },
    legend: {
        enabled: true,
        itemStyle: {
            font: '9pt',
            color: 'white'
        },
        itemHoverStyle: {
            color: 'gray'
        }
    },
    lang: {
        decimalPoint: '.',
        thousandsSep: ','
    }
});