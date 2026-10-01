import { useEffect, useRef } from 'react'
import { CandlestickSeries, ColorType, createChart, HistogramSeries, type Time } from 'lightweight-charts'
import type { Candle } from '../types'

export function MarketChart({ data }: { data: Candle[] }) {
  const host = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!host.current) return
    const chart = createChart(host.current, {
      autoSize: true,
      layout: { background: { type: ColorType.Solid, color: '#0e1116' }, textColor: '#77808d', fontFamily: 'IBM Plex Mono, monospace', fontSize: 10 },
      grid: { vertLines: { color: '#181d24' }, horzLines: { color: '#181d24' } },
      rightPriceScale: { borderColor: '#252b35', scaleMargins: { top: .08, bottom: .24 } },
      timeScale: { borderColor: '#252b35', timeVisible: true, secondsVisible: false, rightOffset: 3 },
      crosshair: { vertLine: { color: '#59616c', labelBackgroundColor: '#333a44' }, horzLine: { color: '#59616c', labelBackgroundColor: '#333a44' } },
      handleScroll: true,
      handleScale: true,
    })
    const candleSeries = chart.addSeries(CandlestickSeries, { upColor: '#b6f34b', downColor: '#ff6b5e', wickUpColor: '#b6f34b', wickDownColor: '#ff6b5e', borderVisible: false })
    candleSeries.setData(data.map(c => ({ ...c, time: c.time as Time })))
    const volume = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' }, priceScaleId: 'volume' })
    volume.priceScale().applyOptions({ scaleMargins: { top: .82, bottom: 0 } })
    volume.setData(data.map(c => ({ time: c.time as Time, value: c.volume, color: c.close >= c.open ? '#b6f34b38' : '#ff6b5e38' })))
    chart.timeScale().fitContent()
    return () => chart.remove()
  }, [data])
  return <div className="chart-wrap"><div ref={host} className="chart" aria-label="Candlestick and volume chart" /><span className="chart-watermark">MARKETLAB · 1m</span></div>
}
