module.exports = {
  content: ['./index.html', './src/**/*.{vue,ts}'],
  theme: {
    extend: {
      fontFamily: {
        display: ['Segoe UI Variable Display', 'Segoe UI', 'sans-serif'],
        body: ['Segoe UI Variable Text', 'Segoe UI', 'sans-serif']
      },
      boxShadow: {
        glass: '0 8px 30px rgba(15, 23, 42, 0.18)'
      },
      backdropBlur: {
        xs: '2px'
      }
    }
  },
  plugins: []
}
