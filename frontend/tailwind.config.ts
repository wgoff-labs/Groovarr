import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        accent: '#10b981',
        danger: '#ef4444',
      },
    },
  },
  plugins: [],
};
export default config;
