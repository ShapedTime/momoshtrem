import type { Config } from 'tailwindcss';

const config: Config = {
  content: [
    './src/pages/**/*.{js,ts,jsx,tsx,mdx}',
    './src/components/**/*.{js,ts,jsx,tsx,mdx}',
    './src/app/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        // Background layers (darkest to lightest)
        'bg-base': '#000000',
        'bg-primary': '#0a0a0a',
        'bg-elevated': '#141414',
        'bg-hover': '#1f1f1f',
        'bg-active': '#292929',
        // Text
        'text-primary': '#ffffff',
        'text-secondary': '#b3b3b3',
        'text-muted': '#666666',
        'text-disabled': '#404040',
        // Accent colors
        'accent-red': '#e50914',
        'accent-blue': '#0071e3',
        'accent-green': '#46d369',
        'accent-yellow': '#f5a623',
        // Semantic
        border: '#333333',
        'focus-ring': '#0071e3',
      },
      fontFamily: {
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'sans-serif',
        ],
      },
      boxShadow: {
        card: '0 8px 16px rgba(0, 0, 0, 0.4)',
        modal: '0 25px 50px rgba(0, 0, 0, 0.5)',
        dropdown: '0 4px 12px rgba(0, 0, 0, 0.3)',
      },
      animation: {
        'fade-in': 'fadeIn 200ms ease-out',
        'spin': 'spin 1s linear infinite',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
      },
    },
  },
  plugins: [],
};

export default config;
