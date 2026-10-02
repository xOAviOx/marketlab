import { CandlestickSeries, ColorType, createChart, createSeriesMarkers, HistogramSeries, type IChartApi, type ISeriesApi, type Time } from 'lightweight-charts'
import { useEffect, useRef } from 'react'
import type { Candle, ScenarioEvent } from '../types'

const baseTime = 1_735_689_600

export function MarketChart({ candles, events }: { candles: Candle[]; events: ScenarioEvent[] }) {
  const container = useRef<HTMLDivElement>(null)
  const chartRef = useRef<IChartApi>()
  const seriesRef = useRef<ISeriesApi<'Candlestick'>>()
  const volumeRef = useRef<ISeriesApi<'Histogram'>>()

  useEffect(() => {
    if (!container.current) return
    const chart = createChart(container.current, {
      autoSize: true,
      layout: { background: { type: ColorType.Solid, color: '#111416' }, textColor: '#8d979d', fontFamily: 'Inter, ui-sans-serif, system-ui' },
      grid: { vertLines: { color: '#1c2226' }, horzLines: { color: '#1c2226' } },
      rightPriceScale: { borderColor: '#293035' }, timeScale: { borderColor: '#293035', timeVisible: true, secondsVisible: true },
      crosshair: { vertLine: { color: '#64748b' }, horzLine: { color: '#64748b' } },
    })
    const series = chart.addSeries(CandlestickSeries, { upColor: '#2dd4bf', downColor: '#fb7185', borderVisible: false, wickUpColor: '#2dd4bf', wickDownColor: '#fb7185', priceFormat: { type: 'price', precision: 2, minMove: .01 } })
    const volume = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' }, priceScaleId: '', color: '#334155' })
    volume.priceScale().applyOptions({ scaleMargins: { top: .82, bottom: 0 } })
    chartRef.current = chart; seriesRef.current = series; volumeRef.current = volume
    return () => { chart.remove(); chartRef.current = undefined }
  }, [])

  useEffect(() => {
    const points = candles.map(c => ({ time: (baseTime + Math.floor(c.time / 1000)) as Time, open: c.open / 100, high: c.high / 100, low: c.low / 100, close: c.close / 100 }))
    seriesRef.current?.setData(points)
    volumeRef.current?.setData(candles.map(c => ({ time: (baseTime + Math.floor(c.time / 1000)) as Time, value: c.volume, color: c.close >= c.open ? '#2dd4bf35' : '#fb718535' })))
    if (seriesRef.current && points.length) {
      createSeriesMarkers(seriesRef.current, events.map(event => ({ time: (baseTime + Math.floor(event.logicalTime / 5000) * 5) as Time, position: 'aboveBar' as const, color: '#fbbf24', shape: 'circle' as const, text: event.title })))
      chartRef.current?.timeScale().fitContent()
    }
  }, [candles, events])

  return <div className="chart" ref={container} aria-label="NOVA candlestick and volume chart" />
}
