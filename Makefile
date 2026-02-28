.PHONY: build run test clean migrate-up migrate-down templ

build: templ
	go build -o maileroo cmd/maileroo/main.go

run: build
	./maileroo

test:
	go test ./...

clean:
	rm -f maileroo

templ:
	templ generate

# Placeholder for migrations (if using a tool like golang-migrate)
migrate-up:
	@echo "Running migrations..."

migrate-down:
	@echo "Rolling back migrations..."
