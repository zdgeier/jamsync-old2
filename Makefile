gen:
	mkdir -p gen/go && protoc --proto_path=proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative proto/*.proto

clean:
	rm -r gen/

server:
	go run cmd/server/main.go

buildserver:
	go build -o jams cmd/server/main.go 

client:
	go run cmd/client/main.go

buildclient:
	go build -o jam cmd/client/main.go 

test:
	go test ./...
