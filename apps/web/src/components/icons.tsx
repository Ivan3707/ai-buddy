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
  return <svg {...base} {...props}><rect x="5.5" y="10.5" width="13" height="9.5" rx="2.2" /><path d="M8.5 10.5V8a3.5 3.5 0 0 1 7 0v2.5" /></svg>;
}
export function CheckIcon(props: SVGProps<SVGSVGElement>) {
  return <svg {...base} strokeWidth={2.2} {...props}><path d="m5 12.5 4.5 4.5L19 7.5" /></svg>;
}
export function ChevronIcon({ direction, ...props }: SVGProps<SVGSVGElement> & { direction: 'left' | 'right' }) {
  return <svg {...base} strokeWidth={2.2} {...props}><path d={direction === 'left' ? 'm14.5 6-6 6 6 6' : 'm9.5 6 6 6-6 6'} /></svg>;
}
