import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: '#090b0e', panel: '#0e1116', line: '#222832', muted: '#77808d',
        acid: '#b6f34b', cyan: '#47d7e8', coral: '#ff6b5e', amber: '#f5b942',
      },
      fontFamily: { sans: ['Inter', 'ui-sans-serif', 'system-ui'], mono: ['IBM Plex Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'] },
    },
  },
  plugins: [],
} satisfies Config
