import { useState } from 'react'

export interface StackedBarPoint {
  date: string
  pass: number
  fail: number
}

interface StackedBarChartProps {
  data: StackedBarPoint[]
  ariaLabel: string
  passLabel: string
  failLabel: string
  totalLabel: string
}

function formatShortDate(isoDate: string): string {
  // YYYY-MM-DD -> MM-DD
  if (isoDate.length >= 10) return isoDate.slice(5, 10)
  return isoDate
}

export function StackedBarChart({
  data,
  ariaLabel,
  passLabel,
  failLabel,
  totalLabel,
}: StackedBarChartProps) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null)

  const width = 760
  const height = 228
  const padTop = 14
  const padRight = 14
  const padBottom = 30
  const padLeft = 42
  const innerW = width - padLeft - padRight
  const innerH = height - padTop - padBottom

  const maxTotal = Math.max(
    1,
    ...data.map((d) => d.pass + d.fail),
  )
  // Round Y max up to a clean tick multiple
  const stepRaw = maxTotal / 4
  const mag = Math.pow(10, Math.floor(Math.log10(Math.max(1, stepRaw))))
  const step = Math.max(1, Math.ceil(stepRaw / mag) * mag)
  const yMax = step * 4
  const yTicks = [0, step, step * 2, step * 3, yMax]

  const count = Math.max(1, data.length)
  const slotW = innerW / count
  const barW = Math.max(3, Math.min(22, slotW * 0.68))

  // Choose ~6 evenly spaced X-axis labels
  const maxLabels = 6
  const labelStep = Math.max(1, Math.ceil(data.length / maxLabels))

  const activePoint = hoveredIdx !== null ? data[hoveredIdx] ?? null : null
  const activeXPercent =
    hoveredIdx !== null && data.length > 0
      ? ((padLeft + hoveredIdx * slotW + slotW / 2) / width) * 100
      : 50

  return (
    <div className="space-y-2.5">
      <div className="flex flex-wrap items-center justify-between gap-2 text-[12px] text-[var(--ink-secondary)]">
        <div className="flex items-center gap-4">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 rounded-[2px] bg-[var(--success)]" />
            {passLabel}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 rounded-[2px] bg-[var(--danger)]" />
            {failLabel}
          </span>
        </div>
        {activePoint && (
          <div className="flex flex-wrap items-center gap-2.5 font-mono text-[11.5px] text-[var(--ink-primary)]">
            <span className="font-semibold">{activePoint.date}</span>
            <span className="text-[var(--success)]">
              {passLabel}: {activePoint.pass}
            </span>
            <span className="text-[var(--danger)]">
              {failLabel}: {activePoint.fail}
            </span>
            <span className="text-[var(--ink-muted)]">
              {totalLabel}: {activePoint.pass + activePoint.fail}
            </span>
          </div>
        )}
      </div>

      <div className="relative w-full">
        <svg
          role="img"
          aria-label={ariaLabel}
          viewBox={`0 0 ${width} ${height}`}
          className="h-auto w-full select-none overflow-visible"
          onMouseLeave={() => setHoveredIdx(null)}
        >
          {/* Horizontal gridlines + Y labels */}
          {yTicks.map((tick) => {
            const y = padTop + innerH - (tick / yMax) * innerH
            return (
              <g key={tick}>
                <line
                  x1={padLeft}
                  x2={width - padRight}
                  y1={y}
                  y2={y}
                  stroke="var(--border-hairline)"
                  strokeDasharray={tick === 0 ? undefined : '3 3'}
                  strokeWidth={1}
                />
                <text
                  x={padLeft - 8}
                  y={y + 3.5}
                  textAnchor="end"
                  fill="var(--ink-muted)"
                  fontSize="10.5"
                  fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
                >
                  {tick}
                </text>
              </g>
            )
          })}

          {/* Bars */}
          {data.map((point, idx) => {
            const total = point.pass + point.fail
            const cx = padLeft + idx * slotW + slotW / 2
            const x = cx - barW / 2
            const passH = (point.pass / yMax) * innerH
            const failH = (point.fail / yMax) * innerH
            const passY = padTop + innerH - passH
            const failY = passY - failH
            const isHovered = hoveredIdx === idx

            return (
              <g
                key={point.date}
                onMouseEnter={() => setHoveredIdx(idx)}
                onFocus={() => setHoveredIdx(idx)}
                onBlur={() => setHoveredIdx(null)}
                tabIndex={0}
                className="cursor-pointer outline-none"
              >
                {/* Invisible hit area for easier hover */}
                <rect
                  x={padLeft + idx * slotW}
                  y={padTop}
                  width={slotW}
                  height={innerH}
                  fill={isHovered ? 'var(--surface-subtle)' : 'transparent'}
                  opacity={isHovered ? 0.55 : 0}
                />
                {passH > 0 && (
                  <rect
                    x={x}
                    y={passY}
                    width={barW}
                    height={Math.max(1, passH)}
                    rx={failH > 0 ? 0 : 2}
                    fill="var(--success)"
                    opacity={hoveredIdx === null || isHovered ? 0.92 : 0.45}
                  />
                )}
                {failH > 0 && (
                  <rect
                    x={x}
                    y={failY}
                    width={barW}
                    height={Math.max(2, failH)}
                    rx={2}
                    fill="var(--danger)"
                    opacity={hoveredIdx === null || isHovered ? 0.96 : 0.5}
                  />
                )}
                {total === 0 && (
                  <rect
                    x={x}
                    y={padTop + innerH - 1.5}
                    width={barW}
                    height={1.5}
                    fill="var(--border-subtle)"
                  />
                )}
              </g>
            )
          })}

          {/* X-axis date labels */}
          {data.map((point, idx) => {
            const isLast = idx === data.length - 1
            if (idx % labelStep !== 0 && !isLast) return null
            if (isLast && data.length > 2 && (data.length - 1) % labelStep < labelStep * 0.5) {
              return null
            }
            const cx = padLeft + idx * slotW + slotW / 2
            return (
              <text
                key={`x-${point.date}`}
                x={cx}
                y={height - 8}
                textAnchor="middle"
                fill="var(--ink-muted)"
                fontSize="10.5"
                fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
              >
                {formatShortDate(point.date)}
              </text>
            )
          })}
        </svg>

        {/* Floating tooltip on hover */}
        {activePoint && (
          <div
            className="pointer-events-none absolute top-1 z-10 -translate-x-1/2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] px-2.5 py-1.5 text-[11.5px] shadow-md"
            style={{
              left: `${Math.max(14, Math.min(86, activeXPercent))}%`,
            }}
          >
            <div className="font-mono font-semibold text-[var(--ink-primary)]">
              {activePoint.date}
            </div>
            <div className="mt-0.5 flex items-center gap-3 font-mono text-[11px]">
              <span className="text-[var(--success)]">
                {passLabel}: {activePoint.pass}
              </span>
              <span className="text-[var(--danger)]">
                {failLabel}: {activePoint.fail}
              </span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
