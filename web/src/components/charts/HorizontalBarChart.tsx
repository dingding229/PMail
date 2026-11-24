export interface HorizontalBarItem {
  label: string
  count: number
  passRate: number
}

interface HorizontalBarChartProps {
  items: HorizontalBarItem[]
  ariaLabel: string
}

export function HorizontalBarChart({ items, ariaLabel }: HorizontalBarChartProps) {
  const maxCount = Math.max(1, ...items.map((item) => item.count))
  const rowH = 38
  const padY = 6
  const width = 460
  const height = Math.max(48, items.length * rowH + padY * 2)
  const labelW = 110
  const metaW = 104
  const barAreaW = width - labelW - metaW

  return (
    <svg
      role="img"
      aria-label={ariaLabel}
      viewBox={`0 0 ${width} ${height}`}
      className="h-auto w-full select-none"
    >
      {items.map((item, idx) => {
        const y = padY + idx * rowH
        const barW = Math.max(4, (item.count / maxCount) * barAreaW)
        const rateText = `${item.passRate.toFixed(1)}%`
        const barFill = item.passRate < 98 ? 'var(--warning)' : 'var(--accent)'

        return (
          <g key={item.label} transform={`translate(0, ${y})`}>
            <text
              x={0}
              y={22}
              fill="var(--ink-primary)"
              fontSize="12.5"
              fontWeight="500"
            >
              {item.label.length > 14 ? `${item.label.slice(0, 13)}…` : item.label}
            </text>

            {/* Track */}
            <rect
              x={labelW}
              y={10}
              width={barAreaW}
              height={16}
              rx={4}
              fill="var(--surface-subtle)"
            />

            {/* Value bar */}
            <rect
              x={labelW}
              y={10}
              width={barW}
              height={16}
              rx={4}
              fill={barFill}
              opacity={0.9}
            />

            {/* Count & pass rate */}
            <text
              x={width}
              y={22}
              textAnchor="end"
              fill="var(--ink-secondary)"
              fontSize="11.5"
              fontFamily="ui-monospace, SFMono-Regular, Menlo, monospace"
            >
              {`${item.count} · ${rateText}`}
            </text>
          </g>
        )
      })}
    </svg>
  )
}
