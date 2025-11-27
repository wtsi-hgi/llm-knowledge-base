/*
  Minimal shadcn-style Button component (Tailwind required).
  This mirrors the structure used by shadcn/ui but keeps the implementation simple.
*/
import * as React from 'react';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
	variant?: 'default' | 'ghost';
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
	({ className = '', variant = 'default', children, ...props }, ref) => {
		const base =
			'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none';

		const variants: Record<string, string> = {
			default: 'bg-slate-900 text-white hover:bg-slate-800 px-4 py-2',
			ghost: 'bg-transparent text-slate-900 px-3 py-1',
		};

		return (
			<button ref={ref} className={[base, variants[variant] ?? variants.default, className].join(' ')} {...props}>
				{children}
			</button>
		);
	}
);

Button.displayName = 'Button';

export default Button;
