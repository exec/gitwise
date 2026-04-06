import { useState } from "react";

interface ContributionDay {
  date: string;
  count: number;
}

interface ContributionGraphProps {
  data: ContributionDay[];
}

function getColor(count: number): string {
  if (count === 0) return "var(--contrib-empty, #161b22)";
  if (count <= 3) return "var(--contrib-low, #0e4429)";
  if (count <= 7) return "var(--contrib-med, #006d32)";
  return "var(--contrib-high, #26a641)";
}

function buildGrid(data: ContributionDay[]): (ContributionDay | null)[][] {
  const lookup = new Map<string, number>();
  for (const d of data) {
    lookup.set(d.date, d.count);
  }

  const today = new Date();
  const end = new Date(today);
  end.setHours(0, 0, 0, 0);

  // Go back ~1 year to the nearest Sunday
  const start = new Date(end);
  start.setDate(start.getDate() - 364);
  while (start.getDay() !== 0) {
    start.setDate(start.getDate() - 1);
  }

  const weeks: (ContributionDay | null)[][] = [];
  let week: (ContributionDay | null)[] = [];

  const cursor = new Date(start);
  while (cursor <= end) {
    const dateStr = cursor.toISOString().slice(0, 10);
    week.push({ date: dateStr, count: lookup.get(dateStr) ?? 0 });
    if (week.length === 7) {
      weeks.push(week);
      week = [];
    }
    cursor.setDate(cursor.getDate() + 1);
  }
  if (week.length > 0) {
    while (week.length < 7) week.push(null);
    weeks.push(week);
  }

  return weeks;
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export default function ContributionGraph({ data }: ContributionGraphProps) {
  const [tooltip, setTooltip] = useState<{ text: string; x: number; y: number } | null>(null);
  const weeks = buildGrid(data);

  const cellSize = 10;
  const cellGap = 2;
  const step = cellSize + cellGap;
  const labelW = 28;
  const headerH = 16;
  const svgW = labelW + weeks.length * step;
  const svgH = headerH + 7 * step;

  // Month labels
  const monthLabels: { text: string; x: number }[] = [];
  let lastMonth = -1;
  for (let w = 0; w < weeks.length; w++) {
    const firstDay = weeks[w].find((d) => d !== null);
    if (firstDay) {
      const month = new Date(firstDay.date).getMonth();
      if (month !== lastMonth) {
        monthLabels.push({ text: MONTHS[month], x: labelW + w * step });
        lastMonth = month;
      }
    }
  }

  const total = data.reduce((sum, d) => sum + d.count, 0);

  return (
    <div className="contribution-graph">
      <h3 className="contribution-graph-title">
        {total} contributions in the last year
      </h3>
      <div className="contribution-graph-scroll">
        <svg
          viewBox={`0 0 ${svgW} ${svgH}`}
          width="100%"
          onMouseLeave={() => setTooltip(null)}
        >
          {/* Month labels */}
          {monthLabels.map((m, i) => (
            <text
              key={i}
              x={m.x}
              y={10}
              fill="var(--text-muted)"
              fontSize="10"
              fontFamily="var(--font-sans)"
            >
              {m.text}
            </text>
          ))}

          {/* Day labels */}
          {[1, 3, 5].map((d) => (
            <text
              key={d}
              x={0}
              y={headerH + d * step + cellSize - 2}
              fill="var(--text-muted)"
              fontSize="9"
              fontFamily="var(--font-sans)"
            >
              {DAYS[d]}
            </text>
          ))}

          {/* Cells */}
          {weeks.map((week, wi) =>
            week.map((day, di) => {
              if (!day) return null;
              const x = labelW + wi * step;
              const y = headerH + di * step;
              return (
                <rect
                  key={day.date}
                  x={x}
                  y={y}
                  width={cellSize}
                  height={cellSize}
                  rx={2}
                  fill={getColor(day.count)}
                  onMouseEnter={(e) => {
                    const rect = (e.target as SVGRectElement).getBoundingClientRect();
                    setTooltip({
                      text: `${day.count} contribution${day.count !== 1 ? "s" : ""} on ${day.date}`,
                      x: rect.left + rect.width / 2,
                      y: rect.top,
                    });
                  }}
                  onMouseLeave={() => setTooltip(null)}
                />
              );
            }),
          )}
        </svg>
      </div>
      {tooltip && (
        <div
          className="contribution-tooltip"
          style={{
            position: "fixed",
            left: tooltip.x,
            top: tooltip.y - 32,
            transform: "translateX(-50%)",
          }}
        >
          {tooltip.text}
        </div>
      )}
    </div>
  );
}
