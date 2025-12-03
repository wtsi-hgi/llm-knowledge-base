import * as React from 'react';
import { cn } from '../../lib/utils';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'default' | 'outline' | 'ghost' | 'link';
  size?: 'default' | 'sm' | 'lg';
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'default', ...props }, ref) => {
    const base = 'inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none';
    const variants: Record<string, string> = {
      default: 'bg-primary text-white hover:bg-primary/90',
      outline: 'border border-input bg-transparent',
      ghost: 'bg-transparent',
      link: 'underline text-primary'
    };
    const sizes: Record<string, string> = {
      default: 'h-10 py-2 px-4',
      sm: 'h-8 px-3',
      lg: 'h-12 px-6'
    };

    return (
      <button ref={ref} className={cn(base, variants[variant], sizes[size], className)} {...props} />
    );
  }
);

Button.displayName = 'Button';

export default Button;
