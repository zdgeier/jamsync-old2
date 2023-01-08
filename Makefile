gen:
	mkdir -p gen/go && protoc --proto_path=proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/*.proto

clean:
	rm -r build

web:
	cd cmd/web/; JAMENV=local go run main.go

webprod:
	cd cmd/web/; JAMENV=prod go run main.go --useenv

buildweb:
	go build -o build/jamweb cmd/web/main.go; cp -R cmd/web/static build; cp -R cmd/web/template build; 

server:
	JAMENV=local go run cmd/server/main.go

buildserver:
	go build -o build/jamserver cmd/server/main.go 

client:
	JAMENV=local go run cmd/client/main.go 

buildclient:
	go build -o build/jam cmd/client/main.go 

buildclients:
	./buildclients.sh

installclient:
	cp build/jam ~/bin/jam

build: clean buildserver buildclients buildweb

test:
	go test ./...
