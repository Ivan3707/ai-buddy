type Props = { percent: number; label: string; size?: number };

/** Кольцо прогресса: светлая дорожка и тёмная дуга выполненной части. */
export function ProgressRing({ percent, label, size = 200 }: Props) {
  const value = Math.max(0, Math.min(100, Math.round(percent)));
  const stroke = size >= 200 ? 16 : size * 0.09; // в макете у большого кольца обводка 16 px
  const radius = (size - stroke) / 2;
  // Дуга выполненной части в макете (Figma arc): старт 0°, внутренний радиус 77,99 % — она толще белой дорожки
  const bold = size >= 200;
  const valueStroke = bold ? (size / 2) * (1 - 0.7799) : stroke;
  const valueRadius = size / 2 - valueStroke / 2; // внешний край совпадает с краем дорожки
  const circumference = 2 * Math.PI * valueRadius;
  return (
    <div
      className="ring"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value}
      style={{ width: size, height: size }}
    >
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden="true">
        <circle className="ring-track" cx={size / 2} cy={size / 2} r={radius} strokeWidth={stroke} />
        <circle
          className="ring-value"
          cx={size / 2}
          cy={size / 2}
          r={valueRadius}
          strokeWidth={valueStroke}
          strokeDasharray={`${(circumference * value) / 100} ${circumference}`}
          transform={bold ? undefined : `rotate(-90 ${size / 2} ${size / 2})`}
        />
      </svg>
      <span className="ring-text">{value}%</span>
    </div>
  );
}
