import type { SVGProps } from 'react';

const base = { width: 24, height: 24, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round', strokeLinejoin: 'round', 'aria-hidden': true } as const;

export function LocationIcon(props: SVGProps<SVGSVGElement>) {
  return <svg {...base} {...props}><path d="M12 21s-6.5-5.6-6.5-11a6.5 6.5 0 0 1 13 0c0 5.4-6.5 11-6.5 11Z" /><circle cx="12" cy="10" r="2.4" /></svg>;
}
export function UserIcon(props: SVGProps<SVGSVGElement>) {
  return <svg {...base} {...props}><circle cx="12" cy="8" r="3.6" /><path d="M4.5 20c.8-3.6 3.7-5.5 7.5-5.5s6.7 1.9 7.5 5.5" /></svg>;
}
export function PencilIcon(props: SVGProps<SVGSVGElement>) {
  return <svg {...base} {...props}><path d="m4 20 1-4L16.5 4.5a2.1 2.1 0 0 1 3 3L8 19l-4 1Z" /><path d="M14.5 6.5l3 3" /></svg>;
}
export function LockIcon(props: SVGProps<SVGSVGElement>) {
  // Значок из макета: закрытый замок #D9D9D9
  return (
    <svg width={24} height={24} viewBox="0 0 24 24" fill="none" aria-hidden="true" {...props}>
      <path d="M6 5C6 2.79086 7.79086 1 10 1H14C16.2091 1 18 2.79086 18 5V7H19C20.6569 7 22 8.34315 22 10V20C22 21.6569 20.6569 23 19 23H5C3.34315 23 2 21.6569 2 20V10C2 8.34315 3.34315 7 5 7H6V5ZM16 5C16 3.89543 15.1046 3 14 3H10C8.89543 3 8 3.89543 8 5V7H16V5ZM5 9C4.44772 9 4 9.44772 4 10V20C4 20.5523 4.44772 21 5 21H19C19.5523 21 20 20.5523 20 20V10C20 9.44772 19.5523 9 19 9H5ZM12 17C13.3807 17 14.5 15.8807 14.5 14.5C14.5 13.1193 13.3807 12 12 12C10.6193 12 9.5 13.1193 9.5 14.5C9.5 15.8807 10.6193 17 12 17Z" fill="#D9D9D9" />
    </svg>
  );
}
export function CheckIcon(props: SVGProps<SVGSVGElement>) {
  // Галочка выполненной задачи из макета: 28×23, заливка белая
  return (
    <svg width="28" height="23" viewBox="0 0 28 23" fill="none" aria-hidden="true" {...props}>
      <path d="M27.4055 0.475588C28.1453 1.15737 28.2024 2.32094 27.5331 3.07449L10.3718 22.3945C10.0399 22.7682 9.57197 22.9868 9.07737 22.9994C8.58277 23.012 8.10475 22.8174 7.7549 22.4611L0.529097 15.1011C-0.176366 14.3825 -0.176366 13.2175 0.529097 12.4989C1.23456 11.7804 2.37834 11.7804 3.0838 12.4989L8.96679 18.4912L24.854 0.605533C25.5233 -0.148018 26.6657 -0.206196 27.4055 0.475588Z" fill="white" />
    </svg>
  );
}
export function ChevronIcon({ direction, ...props }: SVGProps<SVGSVGElement> & { direction: 'left' | 'right' }) {
  // Стрелки календаря из макета: 8×14, заливка #FFFBFB
  return (
    <svg width={8} height={14} viewBox="0 0 8 14" fill="none" aria-hidden="true" {...props}>
      <path
        d={direction === 'left'
          ? 'M7.70711 0.292893C8.09763 0.683417 8.09763 1.31658 7.70711 1.70711L2.41421 7L7.70711 12.2929C8.09763 12.6834 8.09763 13.3166 7.70711 13.7071C7.31658 14.0976 6.68342 14.0976 6.29289 13.7071L0.292893 7.70711C-0.097631 7.31658 -0.0976311 6.68342 0.292893 6.29289L6.29289 0.292893C6.68342 -0.097631 7.31658 -0.0976311 7.70711 0.292893Z'
          : 'M0.292893 0.292893C0.683417 -0.0976311 1.31658 -0.0976311 1.70711 0.292893L7.70711 6.29289C8.09763 6.68342 8.09763 7.31658 7.70711 7.70711L1.70711 13.7071C1.31658 14.0976 0.683417 14.0976 0.292893 13.7071C-0.0976311 13.3166 -0.0976311 12.6834 0.292893 12.2929L5.58579 7L0.292893 1.70711C-0.0976311 1.31658 -0.0976311 0.683417 0.292893 0.292893Z'}
        fill="#FFFBFB"
      />
    </svg>
  );
}
