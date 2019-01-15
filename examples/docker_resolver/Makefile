build: clean
	go build -o server server.go

up: emissary build
	docker-compose up --build

emissary:
	cd ../../ && $(MAKE) build

clean:
	rm -f
