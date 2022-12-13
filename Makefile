gen:
	mkdir -p gen/go && protoc --proto_path=proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/*.proto

clean:
	rm -r gen/

web:
	cd cmd/web/; go run main.go

buildweb:
	go build -o build/jamweb cmd/web/main.go; cp -R cmd/web/static build; cp -R cmd/web/template build; 

server:
	go run cmd/server/main.go

buildserver:
	go build -o build/jamserver cmd/server/main.go 

client:
	go run cmd/client/main.go

buildclient:
	go build -o build/jam cmd/client/main.go 

build: buildserver buildclient buildweb

test:
	go test ./...
