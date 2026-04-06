package maileroo

// Generate Templ components
//go:generate templ generate

// Build Tailwind CSS
//go:generate tailwindcss -c ./tailwind.config.js -o ./static/css/output.css --minify
//go:generate sh -c "printf '.scrollbar-hide{scrollbar-width:none;-ms-overflow-style:none}.scrollbar-hide::-webkit-scrollbar{display:none}' >> ./static/css/output.css"
