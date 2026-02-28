package maileroo

// Generate Templ components
//go:generate templ generate

// Build Tailwind CSS
//go:generate tailwindcss -c ./tailwind.config.js -o ./static/css/output.css --minify
