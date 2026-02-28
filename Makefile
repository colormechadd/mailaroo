.PHONY: build run test clean migrate-up migrate-down templ

build: generate
	go build -o maileroo cmd/maileroo/*.go

run: build
	./maileroo serve

test:
	go test ./...

clean:
	rm -f maileroo static/css/output.css
	find . -name "*_templ.go" -delete

generate:
	go generate ./...

tailwind-watch:
	tailwindcss -c ./tailwind.config.js -i ./static/css/input.css -o ./static/css/output.css --watch

# Placeholder for migrations (if using a tool like golang-migrate)
migrate-up:
	@echo "Running migrations..."
	dbmate up

migrate-down:
	@echo "Rolling back migrations..."
	dbmate down